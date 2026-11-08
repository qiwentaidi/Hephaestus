// Package agentupdate coordinates drained, supervised external-node updates.
package agentupdate

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hephaestus-backend/pkg/agentrelease"
	"hephaestus-backend/pkg/database"
)

const Protocol = 1

var ErrCancelled = errors.New("update cancelled")

func Active(status string) bool {
	switch status {
	case "queued", "waiting", "downloading", "verifying", "restarting", "confirming", "rolling_back":
		return true
	}
	return false
}

func CanCancel(status string) bool {
	return status == "queued" || status == "waiting" || status == "downloading" || status == "verifying"
}

func Supported(node database.ScanAgentNode) bool {
	if node.LocalWorker || node.UpdateProtocol != Protocol {
		return false
	}
	for _, p := range agentrelease.Current().Packages {
		if p.OS == node.UpdateOS && p.Arch == node.UpdateArch {
			return true
		}
	}
	return false
}

// Request validates the entire batch atomically. Updating never changes the
// user's enabled preference. A node row serializes this with both task claimers.
func Request(db *gorm.DB, ids []string, target string) error {
	if len(ids) == 0 || len(ids) > 100 {
		return fmt.Errorf("select 1–100 nodes")
	}
	ids = append([]string(nil), ids...)
	sort.Strings(ids)
	return db.Transaction(func(tx *gorm.DB) error {
		for i, id := range ids {
			if id == "" || (i > 0 && ids[i-1] == id) {
				return fmt.Errorf("invalid or duplicate node")
			}
			var node database.ScanAgentNode
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&node, "agent_id=?", id).Error; err != nil {
				return err
			}
			if !Supported(node) {
				return fmt.Errorf("%s: install a supervised Agent before using platform updates", id)
			}
			if node.Retired || node.Status != "online" || time.Since(node.LastHeartbeat) > 45*time.Second {
				return fmt.Errorf("%s: node is offline", id)
			}
			if node.AgentVersion == target {
				return fmt.Errorf("%s: version already matches", id)
			}
			var old database.AgentUpdate
			err := tx.First(&old, "agent_id=?", id).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if Active(old.Status) {
				return fmt.Errorf("%s: update already pending", id)
			}
			now := time.Now()
			job := database.AgentUpdate{AgentID: id, RequestID: uuid.NewString(), TargetVersion: target, PreviousVersion: node.AgentVersion, Status: "queued", RequestedAt: now, UpdatedAt: now}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "agent_id"}}, UpdateAll: true}).Create(&job).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func Cancel(db *gorm.DB, id, requestID string) error {
	return change(db, id, requestID, func(tx *gorm.DB, job *database.AgentUpdate) error {
		if !CanCancel(job.Status) {
			return fmt.Errorf("update can no longer be cancelled")
		}
		if job.Status == "queued" {
			job.Status = "cancelled"
		} else {
			job.CancelRequested = true
		}
		return nil
	})
}

// change always locks node before update; claimers use the same node lock.
func change(db *gorm.DB, id, requestID string, fn func(*gorm.DB, *database.AgentUpdate) error) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var node database.ScanAgentNode
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&node, "agent_id=?", id).Error; err != nil {
			return err
		}
		var job database.AgentUpdate
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&job, "agent_id=? AND request_id=?", id, requestID).Error; err != nil {
			return err
		}
		if err := fn(tx, &job); err != nil {
			return err
		}
		job.UpdatedAt = time.Now()
		return tx.Save(&job).Error
	})
}

func SetState(db *gorm.DB, job database.AgentUpdate, status string, progress int) error {
	allowed := map[string][]string{
		"queued": {"waiting"}, "waiting": {"waiting", "downloading"}, "downloading": {"downloading", "verifying"},
		"verifying": {"restarting"}, "restarting": {"confirming", "rolling_back"}, "confirming": {"confirming", "rolling_back"}, "rolling_back": {"rolling_back"},
	}
	return change(db, job.AgentID, job.RequestID, func(tx *gorm.DB, current *database.AgentUpdate) error {
		if !Active(current.Status) {
			return fmt.Errorf("request is no longer active")
		}
		if current.CancelRequested {
			return ErrCancelled
		}
		valid := false
		for _, next := range allowed[current.Status] {
			if next == status {
				valid = true
			}
		}
		if !valid {
			return fmt.Errorf("invalid update transition %s -> %s", current.Status, status)
		}
		if status == "waiting" {
			if err := tx.Model(&database.ScanAgentNode{}).Where("agent_id=?", job.AgentID).Update("update_pending", true).Error; err != nil {
				return err
			}
		}
		current.Status, current.Progress = status, progress
		return nil
	})
}

func Finish(db *gorm.DB, job database.AgentUpdate, status, message string) error {
	return change(db, job.AgentID, job.RequestID, func(tx *gorm.DB, current *database.AgentUpdate) error {
		if !Active(current.Status) {
			return nil
		}
		current.Status, current.Error = status, message
		if status == "completed" {
			current.Progress = 100
		}
		return tx.Model(&database.ScanAgentNode{}).Where("agent_id=?", job.AgentID).Update("update_pending", false).Error
	})
}

func Check(db *gorm.DB, job database.AgentUpdate) error {
	var current database.AgentUpdate
	if err := db.First(&current, "agent_id=? AND request_id=?", job.AgentID, job.RequestID).Error; err != nil {
		return err
	}
	if !Active(current.Status) || current.CancelRequested {
		return ErrCancelled
	}
	return nil
}

// StopRequested asks the child to drain its real worker goroutines. Database
// counts alone never acknowledge process exit or trigger executable replacement.
func StopRequested(db *gorm.DB, id string) bool {
	var n int64
	return db.Model(&database.AgentUpdate{}).Where("agent_id=? AND status='restarting'", id).Count(&n).Error == nil && n > 0
}

func Running(db *gorm.DB, id string) (int64, error) {
	var scan, ai int64
	err := db.Model(&database.VulnScanTaskItem{}).Where("worker_agent_id=? AND status='running' AND lease_expires_at>?", id, time.Now()).Count(&scan).Error
	if err != nil {
		return 0, err
	}
	err = db.Table("pentest_worker_turns").Where("node_id=? AND status IN ('running','cancelling') AND lease_expires_at>?", id, time.Now()).Count(&ai).Error
	return scan + ai, err
}
