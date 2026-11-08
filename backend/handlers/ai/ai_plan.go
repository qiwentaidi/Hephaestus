package ai

import (
	"errors"
	"net/http"
	"strings"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/lib/pentest/taskengine"
	aipkg "hephaestus-backend/pkg/ai"

	"github.com/gin-gonic/gin"
)

type createPlanRunRequest struct {
	ProjectID         string   `json:"projectId"`
	Objective         string   `json:"objective"`
	Targets           []string `json:"targets"`
	ProjectName       string   `json:"projectName"`
	Scope             []string `json:"scope"`
	PlannerProviderID string   `json:"plannerProviderId"`
	PlannerModel      string   `json:"plannerModel"`
	WorkerProviderID  string   `json:"workerProviderId"`
	WorkerModel       string   `json:"workerModel"`
	MaxWorkers        int      `json:"maxWorkers"`
	MaxTotalWorkers   int      `json:"maxTotalWorkers"`
}

func CreateAIPlanRunHandler(c *gin.Context) { createARTEXPlan(c) }

func ReopenAIPlanRunHandler(c *gin.Context) {
	var req struct {
		GoalIDs            []int64 `json:"goalIds"`
		IntentIDs          []int64 `json:"intentIds"`
		AcceptanceCriteria string  `json:"acceptanceCriteria"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		nativeError(c, http.StatusBadRequest, err)
		return
	}
	ctx := c.Request.Context()
	store, err := aipkg.StoreFromAppDB(ctx)
	if err != nil {
		nativeError(c, http.StatusInternalServerError, err)
		return
	}
	run, err := store.GetPlanRun(ctx, c.Param("id"))
	if err != nil {
		nativeError(c, http.StatusNotFound, err)
		return
	}
	if !pentest.IsOrchestrationRun(run.ID) {
		nativeError(c, http.StatusConflict, pentest.ErrLegacyOrchestrationReadOnly)
		return
	}
	rt, err := taskengine.Get(ctx)
	if err != nil {
		nativeError(c, http.StatusServiceUnavailable, err)
		return
	}
	if err = rt.Engine.ReopenHostTask(nativeTaskID(run.ID), req.GoalIDs, req.IntentIDs, req.AcceptanceCriteria); err != nil {
		nativeError(c, http.StatusConflict, err)
		return
	}
	projectionErr := syncARTEXRun(ctx, store, rt, run)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"id": run.ID, "paused": true, "projectionPending": projectionErr != nil}})
}

func ResumeAIPlanRunHandler(c *gin.Context) {
	ctx := c.Request.Context()
	store, err := aipkg.StoreFromAppDB(ctx)
	if err != nil {
		nativeError(c, http.StatusInternalServerError, err)
		return
	}
	run, err := store.GetPlanRun(ctx, c.Param("id"))
	if err != nil {
		nativeError(c, http.StatusNotFound, err)
		return
	}
	if !pentest.IsOrchestrationRun(run.ID) {
		nativeError(c, http.StatusConflict, pentest.ErrLegacyOrchestrationReadOnly)
		return
	}
	rt, err := taskengine.Get(ctx)
	if err != nil {
		nativeError(c, http.StatusServiceUnavailable, err)
		return
	}
	if err = rt.Engine.ResumeHostTask(nativeTaskID(run.ID)); err != nil {
		nativeError(c, http.StatusConflict, err)
		return
	}
	// Resume is committed independently of the UI projection. Returning an error
	// after admission would misleadingly suggest that the task never resumed.
	projectionErr := syncARTEXRun(ctx, store, rt, run)
	_, state, err := rt.Engine.HostTask(nativeTaskID(run.ID))
	if err != nil {
		nativeError(c, http.StatusInternalServerError, err)
		return
	}
	data := gin.H{"id": run.ID, "status": nativeRunState(state.Status)}
	if projectionErr != nil {
		data["projectionPending"] = true
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

// PauseAIPlanRunHandler uses the native pause/acknowledgement chain.
func PauseAIPlanRunHandler(c *gin.Context) { cancelAIPlanRun(c) }

// CancelAIPlanRunHandler keeps the old endpoint as a compatibility alias.
func CancelAIPlanRunHandler(c *gin.Context) { cancelAIPlanRun(c) }

func cancelAIPlanRun(c *gin.Context) {
	store, err := aipkg.StoreFromAppDB(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	run, err := store.GetPlanRun(ctx, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
		return
	}
	if pentest.IsOrchestrationRun(run.ID) {
		cancelARTEXPlan(c, store, run)
		return
	}
	nativeError(c, http.StatusConflict, pentest.ErrLegacyOrchestrationReadOnly)
	return
}

func ListAIPlanRunsHandler(c *gin.Context) {
	store, err := aipkg.StoreFromAppDB(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	projectID := strings.TrimSpace(c.Query("projectId"))
	if c.Query("view") == "summary" {
		items, err := store.ListPlanRunSummaries(ctx, projectID, strings.TrimSpace(c.Query("search")), 100)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": items}})
		return
	}
	var runs []pentest.PlanRun
	if projectID != "" {
		runs, err = store.ListPlanRunsByProject(ctx, projectID, 100)
	} else {
		runs, err = store.ListPlanRuns(ctx, 100)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	items := make([]map[string]any, 0, len(runs))
	for _, run := range runs {
		project, _ := store.GetProject(ctx, run.ProjectID)
		runItems, _ := store.ListPlanItems(ctx, run.ID)
		if controller, ok := planControllerConversation(ctx, store, run); ok {
			run.ControllerConversationID = controller.ID
		}
		items = append(items, map[string]any{"run": run, "project": planProjectDTO(project), "items": runItems})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": items}})
}

// ListAIPlanItemRowsHandler reads the background projection without requesting
// a synchronous engine snapshot or materializing every item's report.
func ListAIPlanItemRowsHandler(c *gin.Context) {
	ctx := c.Request.Context()
	store, err := aipkg.StoreFromAppDB(ctx)
	if err != nil {
		nativeError(c, http.StatusInternalServerError, err)
		return
	}
	run, err := store.GetPlanRun(ctx, c.Param("id"))
	if err != nil {
		nativeError(c, http.StatusNotFound, err)
		return
	}
	search := strings.TrimSpace(c.Query("search"))
	items, err := store.ListPlanItemRows(ctx, run.ID, search)
	if err != nil {
		nativeError(c, http.StatusInternalServerError, err)
		return
	}
	data := gin.H{"items": items}
	if search == "" {
		data["summary"] = run.Summary
		data["error"] = run.Error
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func GetAIPlanItemHandler(c *gin.Context) {
	store, err := aipkg.StoreFromAppDB(c.Request.Context())
	if err != nil {
		nativeError(c, http.StatusInternalServerError, err)
		return
	}
	item, err := store.GetPlanItem(c.Request.Context(), c.Param("itemId"))
	if err != nil || item.RunID != c.Param("id") {
		nativeError(c, http.StatusNotFound, errors.New("plan item not found"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": item})
}

func GetAIPlanRunHandler(c *gin.Context) {
	store, err := aipkg.StoreFromAppDB(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	run, err := store.GetPlanRun(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
		return
	}
	if err := syncARTEXConversation(c.Request.Context(), store, run.PlannerConversationID); err != nil {
		nativeError(c, 503, err)
		return
	}
	run, _ = store.GetPlanRun(c.Request.Context(), run.ID)
	snapshot, err := planRunSnapshot(c.Request.Context(), store, run)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": snapshot})
}

// EnsureAIPlanRunControllerHandler opens an existing controller; historical runs
// never acquire a new writable conversation.
func EnsureAIPlanRunControllerHandler(c *gin.Context) {
	store, err := aipkg.StoreFromAppDB(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	run, err := store.GetPlanRun(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
		return
	}
	controller, ok := planControllerConversation(c.Request.Context(), store, run)
	if !ok {
		nativeError(c, 409, pentest.ErrLegacyOrchestrationReadOnly)
		return
	}
	run.ControllerConversationID = controller.ID
	snapshot, err := planRunSnapshot(c.Request.Context(), store, run)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": snapshot})
}
