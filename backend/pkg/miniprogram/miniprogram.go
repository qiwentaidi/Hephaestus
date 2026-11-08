package miniprogram

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ditashi/jsbeautifier-go/jsbeautifier"
	"github.com/yosssi/gohtml"
	"golang.org/x/crypto/pbkdf2"
)

const (
	defaultSalt = "saltiest"
	defaultIV   = "the iv: 16 bytes"

	matchesFileName        = "matches.txt"
	groupedMatchesFileName = "matches.json"
	manifestFileName       = "manifest.json"
	pluginDirName          = "__plugin__"
	rootVersionName        = "root"
	maxFileWorkers         = 8
	maxJSBeautifyBytes     = 256 * 1024
)

var skippedExtractExts = map[string]struct{}{
	".br":     {},
	".gif":    {},
	".jpeg":   {},
	".jpg":    {},
	".png":    {},
	".webp":   {},
	".wxapkg": {},
}

type ExtractionRule struct {
	ID      string `json:"id"`
	Pattern string `json:"pattern"`
	Enabled bool   `json:"enabled"`
}

type RuleGroup struct {
	ID      string           `json:"id"`
	Name    string           `json:"name"`
	Enabled bool             `json:"enabled"`
	Rules   []ExtractionRule `json:"rules"`
}

type SensitiveMatch struct {
	Value   string `json:"value"`
	GroupID string `json:"groupId"`
	RuleID  string `json:"ruleId,omitempty"`
}

type MatchGroup struct {
	ID      string           `json:"id"`
	Name    string           `json:"name"`
	Matches []SensitiveMatch `json:"matches"`
	Count   int              `json:"count"`
}

type GroupedMatchesResult struct {
	Matches []string     `json:"matches"`
	Groups  []MatchGroup `json:"groups"`
	Total   int          `json:"total"`
}

var DefaultRuleGroups = []RuleGroup{
	{
		ID:      "cloud-credentials",
		Name:    "云服务凭证",
		Enabled: true,
		Rules: []ExtractionRule{
			{ID: "access-token", Pattern: `(?i)["']?access[_-]?token["']?[^\S\r\n]*[=:][^\S\r\n]*["']?[\w-]+["']?`, Enabled: true},
			{ID: "access-secret", Pattern: `(?i)["']?access[_-]?secret["']?[^\S\r\n]*[=:][^\S\r\n]*["']?[\w-]+["']?`, Enabled: true},
			{ID: "access-key", Pattern: `(?i)["']?access[_-]?key["']?[^\S\r\n]*[=:][^\S\r\n]*["']?[\w-]+["']?`, Enabled: true},
			{ID: "access-key-id", Pattern: `(?i)["']?access[_-]?key[_-]?id["']?[^\S\r\n]*[=:][^\S\r\n]*["']?[\w-]+["']?`, Enabled: true},
			{ID: "access-key-secret", Pattern: `(?i)["']?access[_-]?key[_-]?secret["']?[^\S\r\n]*[=:][^\S\r\n]*["']?[\w-]+["']?`, Enabled: true},
			{ID: "secret-access-key", Pattern: `(?i)["']?secret[_-]?access[_-]?key["']?[^\S\r\n]*[=:][^\S\r\n]*["']?[\w-]+["']?`, Enabled: true},
			{ID: "secret-access-token", Pattern: `(?i)["']?secret[_-]?access[_-]?token["']?[^\S\r\n]*[=:][^\S\r\n]*["']?[\w-]+["']?`, Enabled: true},
		},
	},
	{
		ID:      "application-credentials",
		Name:    "应用凭证",
		Enabled: true,
		Rules: []ExtractionRule{
			{ID: "app-credential", Pattern: `(?i)["']?app[_-]?(id|secret)["']?[^\S\r\n]*[=:][^\S\r\n]*["']?[\w-]+["']?`, Enabled: true},
			{ID: "corp-credential", Pattern: `(?i)["']?.*corp[_-]?(Id|Secret)["']?[^\S\r\n]*[=:][^\S\r\n]*["']?[\w-]+["']?`, Enabled: true},
			{ID: "session-key", Pattern: `(?i)["']?session[_-]?key["']?[^\S\r\n]*[=:][^\S\r\n]*["']?[\w-]+["']?`, Enabled: true},
			{ID: "api-key", Pattern: `(?i)["']?api[_-]?key["']?[^\S\r\n]*[=:][^\S\r\n]*["']?[\w-]+["']?`, Enabled: true},
			{ID: "api-secret", Pattern: `(?i)["']?api[_-]?secret["']?[^\S\r\n]*[=:][^\S\r\n]*["']?[\w-]+["']?`, Enabled: true},
		},
	},
	{
		ID:      "account-privacy",
		Name:    "账号与隐私",
		Enabled: true,
		Rules: []ExtractionRule{
			{ID: "phone", Pattern: `(?:\+?86)?1[3-9]\d{9}`, Enabled: true},
			{ID: "id-card", Pattern: `\b[1-9]\d{5}(?:18|19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx]\b`, Enabled: true},
			{ID: "username", Pattern: `(?i)(?:["']?user(?:name)?["']?[^\S\r\n]*[=:])[^\S\r\n]*["']?[\p{Han}\w-]+["']?`, Enabled: true},
			{ID: "password", Pattern: `(?i)["']?pass(?:word)?["']?[^\S\n]*[=:][^\S\r\n]*["']?[@#!$%^&*\w-]+["']?`, Enabled: true},
		},
	},
	{
		ID:      "url-endpoints",
		Name:    "URL 与接口",
		Enabled: true,
		Rules: []ExtractionRule{
			{ID: "request-path", Pattern: `(?i)(post|get|delete|put|url)[:\(]+[^\S\r\n]*["']\/(?:[\w-]+\/)*(?:[\w-]+)\/*\??(?:[\w=&-]+)?["']\)*`, Enabled: true},
			{ID: "http-url", Pattern: `(?i)http(s)?://(?:[\w-]+\/)*(?:[\w-]+)\/*\??(?:[\w\=&-]+)?["']\)*`, Enabled: true},
			{ID: "route-path", Pattern: `(?i)["']\/(?:[\w-]+\/)*[\w-]+(.cgi|.php|.action|.jsp|.jspx|.asp|.aspx|.py|.rb|.html|.htm|.tpl|.do|.jsf)?\/*\??(?:[\w=&-]+)?\)*`, Enabled: true},
		},
	},
}

var DefaultRules = FlattenEnabledRulePatterns(DefaultRuleGroups)

type Service struct {
	AppletPath string
	OutputRoot string
	Rules      []string
	RuleGroups []RuleGroup

	matcher *Matcher
}

type MiniProgram struct {
	AppID      string    `json:"appId"`
	UpdateDate string    `json:"updateDate"`
	Info       *AppInfo  `json:"info,omitempty"`
	Versions   []Version `json:"versions"`
}

type AppInfo struct {
	AppID       string `json:"appId"`
	Nickname    string `json:"nickname,omitempty"`
	Username    string `json:"username,omitempty"`
	Description string `json:"description,omitempty"`
	Avatar      string `json:"avatar,omitempty"`
	Source      string `json:"source,omitempty"`
}

type Version struct {
	Number       string `json:"number"`
	UpdateDate   string `json:"updateDate"`
	WxapkgCount  int    `json:"wxapkgCount"`
	Decompiled   bool   `json:"decompiled"`
	OutputDir    string `json:"outputDir,omitempty"`
	MatchedCount int    `json:"matchedCount"`
}

type PackageFile struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type DecompileResult struct {
	AppID        string        `json:"appId"`
	Version      string        `json:"version"`
	OutputDir    string        `json:"outputDir"`
	Packages     []PackageFile `json:"packages"`
	FileCount    int           `json:"fileCount"`
	SampleFiles  []string      `json:"sampleFiles"`
	Matched      []string      `json:"matched"`
	MatchGroups  []MatchGroup  `json:"matchGroups"`
	MatchedCount int           `json:"matchedCount"`
	DurationMs   int64         `json:"durationMs"`
	Errors       []string      `json:"errors"`
}

type wxapkgFile struct {
	Name   string
	Offset uint32
	Size   uint32
}

type localImageCandidate struct {
	data     []byte
	mimeType string
	width    int
	height   int
	size     int64
}

func NewService(appletPath, outputRoot string, rules []string) *Service {
	return NewServiceWithRuleGroups(appletPath, outputRoot, RuleGroupsFromLegacyRules(rules))
}

func NewServiceWithRuleGroups(appletPath, outputRoot string, ruleGroups []RuleGroup) *Service {
	if len(ruleGroups) == 0 {
		ruleGroups = CloneRuleGroups(DefaultRuleGroups)
	}
	ruleGroups = NormalizeRuleGroups(ruleGroups)
	rules := FlattenEnabledRulePatterns(ruleGroups)
	return &Service{
		AppletPath: strings.TrimSpace(appletPath),
		OutputRoot: strings.TrimSpace(outputRoot),
		Rules:      rules,
		RuleGroups: ruleGroups,
		matcher:    NewGroupedMatcher(ruleGroups),
	}
}

func CloneRuleGroups(groups []RuleGroup) []RuleGroup {
	result := make([]RuleGroup, 0, len(groups))
	for _, group := range groups {
		cloned := group
		cloned.Rules = append([]ExtractionRule(nil), group.Rules...)
		result = append(result, cloned)
	}
	return result
}

func FlattenEnabledRulePatterns(groups []RuleGroup) []string {
	patterns := make([]string, 0)
	for _, group := range groups {
		if !group.Enabled {
			continue
		}
		for _, rule := range group.Rules {
			if rule.Enabled {
				patterns = append(patterns, rule.Pattern)
			}
		}
	}
	return removeEmptyDuplicateStrings(patterns)
}

func RuleGroupsFromLegacyRules(rules []string) []RuleGroup {
	rules = removeEmptyDuplicateStrings(rules)
	if len(rules) == 0 {
		return CloneRuleGroups(DefaultRuleGroups)
	}
	if len(rules) == len(DefaultRules) {
		defaultPatterns := make(map[string]struct{}, len(DefaultRules))
		for _, pattern := range DefaultRules {
			defaultPatterns[pattern] = struct{}{}
		}
		matchesDefault := len(defaultPatterns) == len(rules)
		for _, pattern := range rules {
			if _, exists := defaultPatterns[pattern]; !exists {
				matchesDefault = false
				break
			}
		}
		if matchesDefault {
			return CloneRuleGroups(DefaultRuleGroups)
		}
	}

	group := RuleGroup{
		ID:      "legacy-custom",
		Name:    "自定义规则",
		Enabled: true,
		Rules:   make([]ExtractionRule, 0, len(rules)),
	}
	for index, pattern := range rules {
		group.Rules = append(group.Rules, ExtractionRule{
			ID:      fmt.Sprintf("legacy-rule-%d", index+1),
			Pattern: pattern,
			Enabled: true,
		})
	}
	return []RuleGroup{group}
}

func NormalizeRuleGroups(groups []RuleGroup) []RuleGroup {
	result := make([]RuleGroup, 0, len(groups))
	seenGroupIDs := make(map[string]struct{}, len(groups))
	for groupIndex, group := range groups {
		group.ID = uniqueRuleIdentifier(group.ID, fmt.Sprintf("group-%d", groupIndex+1), seenGroupIDs)
		group.Name = strings.TrimSpace(group.Name)
		if group.Name == "" {
			group.Name = fmt.Sprintf("规则组 %d", groupIndex+1)
		}

		seenRuleIDs := make(map[string]struct{}, len(group.Rules))
		seenPatterns := make(map[string]struct{}, len(group.Rules))
		rules := make([]ExtractionRule, 0, len(group.Rules))
		for ruleIndex, rule := range group.Rules {
			rule.Pattern = strings.TrimSpace(rule.Pattern)
			if rule.Pattern == "" {
				continue
			}
			if _, exists := seenPatterns[rule.Pattern]; exists {
				continue
			}
			seenPatterns[rule.Pattern] = struct{}{}
			rule.ID = uniqueRuleIdentifier(rule.ID, fmt.Sprintf("rule-%d", ruleIndex+1), seenRuleIDs)
			rules = append(rules, rule)
		}
		group.Rules = rules
		result = append(result, group)
	}
	return result
}

func uniqueRuleIdentifier(value, fallback string, seen map[string]struct{}) string {
	base := strings.TrimSpace(value)
	if base == "" {
		base = fallback
	}
	candidate := base
	for suffix := 2; ; suffix++ {
		if _, exists := seen[candidate]; !exists {
			seen[candidate] = struct{}{}
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, suffix)
	}
}

func DefaultAppletPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return bestAppletPathCandidate(expandAppletPathCandidates(defaultAppletPathCandidates(homeDir)))
}

func ResolveAppletPath(appletPath string) string {
	appletPath = normalizeAppletPathCandidate(appletPath)
	if appletPath == "" {
		return ""
	}
	if resolved := bestAppletPathCandidate(appletPathCandidates(appletPath)); resolved != "" {
		return resolved
	}
	return appletPath
}

func (s *Service) ScanPrograms() ([]MiniProgram, error) {
	appletPath := s.resolvedAppletPath()
	if appletPath == "" {
		return []MiniProgram{}, nil
	}

	root, err := filepath.Abs(appletPath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []MiniProgram{}, nil
		}
		return nil, err
	}
	sortDirEntriesByModTimeDesc(entries)

	items := make([]MiniProgram, 0)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "wx") {
			continue
		}

		appID := entry.Name()
		appDir := filepath.Join(root, appID)
		versionEntries, err := os.ReadDir(appDir)
		if err != nil {
			continue
		}
		sortDirEntriesByModTimeDesc(versionEntries)

		info, err := entry.Info()
		if err != nil {
			continue
		}

		versions := make([]Version, 0)
		hasRootVersion := false
		if packages, err := s.findRootWxapkgFiles(appID); err == nil && len(packages) > 0 {
			hasRootVersion = true
			outputDir := s.versionOutputDir(appID, rootVersionName)
			matches, _ := s.ReadMatches(appID, rootVersionName)
			versions = append(versions, Version{
				Number:       rootVersionName,
				UpdateDate:   formatModTime(newestPackageModTime(packages, info.ModTime())),
				WxapkgCount:  len(packages),
				Decompiled:   directoryHasFiles(outputDir),
				OutputDir:    outputDir,
				MatchedCount: len(matches),
			})
		}

		for _, versionEntry := range versionEntries {
			if !versionEntry.IsDir() {
				continue
			}

			version := versionEntry.Name()
			if hasRootVersion && version == rootVersionName {
				continue
			}
			packages, err := s.findWxapkgFiles(appID, version)
			if err != nil || len(packages) == 0 {
				continue
			}

			versionInfo, err := versionEntry.Info()
			if err != nil {
				continue
			}

			outputDir := s.versionOutputDir(appID, version)
			matches, _ := s.ReadMatches(appID, version)
			versions = append(versions, Version{
				Number:       version,
				UpdateDate:   formatModTime(versionInfo.ModTime()),
				WxapkgCount:  len(packages),
				Decompiled:   directoryHasFiles(outputDir),
				OutputDir:    outputDir,
				MatchedCount: len(matches),
			})
		}

		if len(versions) == 0 {
			continue
		}
		items = append(items, MiniProgram{
			AppID:      appID,
			UpdateDate: formatModTime(info.ModTime()),
			Info:       s.readLocalAppInfo(appID, versions),
			Versions:   versions,
		})
	}

	return items, nil
}

func (s *Service) Decompile(appID, version string) (*DecompileResult, error) {
	start := time.Now()
	appID = strings.TrimSpace(appID)
	version = strings.TrimSpace(version)
	if appID == "" || version == "" {
		return nil, errors.New("appId 和 version 不能为空")
	}
	if !isSafePathSegment(appID) || !isSafePathSegment(version) {
		return nil, errors.New("appId 或 version 包含非法路径字符")
	}
	appletPath := s.resolvedAppletPath()
	if strings.TrimSpace(appletPath) == "" {
		return nil, errors.New("微信 Applet 路径不能为空")
	}
	if strings.TrimSpace(s.OutputRoot) == "" {
		return nil, errors.New("输出目录不能为空")
	}

	packages, err := s.findWxapkgFiles(appID, version)
	if err != nil {
		return nil, err
	}
	if len(packages) == 0 {
		return nil, errors.New("未找到 wxapkg 文件")
	}

	versionRoot := filepath.Join(appletPath, appID, version)
	if version == rootVersionName {
		versionRoot = filepath.Join(appletPath, appID)
	}
	outputDir := s.versionOutputDir(appID, version)
	if err := os.RemoveAll(outputDir); err != nil {
		return nil, fmt.Errorf("清理旧输出目录失败: %w", err)
	}

	result := &DecompileResult{
		AppID:     appID,
		Version:   version,
		OutputDir: outputDir,
		Packages:  packages,
	}
	allFiles := make([]string, 0)
	for _, pkg := range packages {
		data, err := os.ReadFile(pkg.Path)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", pkg.Name, err.Error()))
			continue
		}
		if shouldDecrypt(data) {
			data, err = DecryptWxapkg(data, appID, defaultSalt, defaultIV)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s 解密失败: %s", pkg.Name, err.Error()))
				continue
			}
		}

		rel, err := filepath.Rel(versionRoot, pkg.Path)
		if err != nil {
			rel = filepath.Base(pkg.Path)
		}
		packageOutputDir := resolvePackageOutputDir(outputDir, rel, pkg.Name)
		files, err := s.UnpackWxapkg(data, packageOutputDir)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s 反编译失败: %s", pkg.Name, err.Error()))
			continue
		}
		allFiles = append(allFiles, files...)
	}
	if len(allFiles) == 0 && len(result.Errors) > 0 {
		return nil, fmt.Errorf("反编译失败: %s", strings.Join(result.Errors, "; "))
	}

	result.FileCount = len(allFiles)
	result.SampleFiles = sampleRelativeFiles(outputDir, allFiles, 200)
	result.MatchGroups = s.ExtractMatchGroups(allFiles)
	result.Matched = flattenMatchGroups(result.MatchGroups)
	result.MatchedCount = len(result.Matched)
	result.DurationMs = time.Since(start).Milliseconds()

	if err := writeResultFiles(outputDir, result); err != nil {
		return nil, err
	}

	return result, nil
}

func (s *Service) Clear(appID, version string) error {
	root := s.OutputRoot
	if strings.TrimSpace(appID) != "" {
		if !isSafePathSegment(appID) {
			return errors.New("appId 包含非法路径字符")
		}
		root = filepath.Join(root, appID)
	}
	if strings.TrimSpace(version) != "" {
		if !isSafePathSegment(version) {
			return errors.New("version 包含非法路径字符")
		}
		root = filepath.Join(root, appID, version)
	}
	if strings.TrimSpace(root) == "" {
		return errors.New("输出目录不能为空")
	}
	return os.RemoveAll(root)
}

func (s *Service) ReadMatches(appID, version string) ([]string, error) {
	if !isSafePathSegment(appID) || !isSafePathSegment(version) {
		return nil, errors.New("appId 或 version 包含非法路径字符")
	}
	path := filepath.Join(s.versionOutputDir(appID, version), matchesFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	return removeEmptyDuplicateStrings(strings.Split(string(data), "\n")), nil
}

func (s *Service) ReadGroupedMatches(appID, version string) (GroupedMatchesResult, error) {
	if !isSafePathSegment(appID) || !isSafePathSegment(version) {
		return GroupedMatchesResult{}, errors.New("appId 或 version 包含非法路径字符")
	}
	path := filepath.Join(s.versionOutputDir(appID, version), groupedMatchesFileName)
	data, err := os.ReadFile(path)
	if err == nil {
		var result GroupedMatchesResult
		if err := json.Unmarshal(data, &result); err != nil {
			return GroupedMatchesResult{}, err
		}
		result.Groups = normalizeMatchGroups(result.Groups)
		result.Matches = flattenMatchGroups(result.Groups)
		result.Total = len(result.Matches)
		return result, nil
	}
	if !os.IsNotExist(err) {
		return GroupedMatchesResult{}, err
	}

	matches, err := s.ReadMatches(appID, version)
	if err != nil {
		return GroupedMatchesResult{}, err
	}
	if len(matches) == 0 {
		groups := emptyMatchGroups(s.RuleGroups)
		return GroupedMatchesResult{Matches: []string{}, Groups: groups, Total: 0}, nil
	}
	legacyMatches := make([]SensitiveMatch, 0, len(matches))
	for _, value := range matches {
		legacyMatches = append(legacyMatches, SensitiveMatch{
			Value:   value,
			GroupID: "unclassified",
		})
	}
	return GroupedMatchesResult{
		Matches: matches,
		Groups: []MatchGroup{{
			ID:      "unclassified",
			Name:    "未分组",
			Matches: legacyMatches,
			Count:   len(legacyMatches),
		}},
		Total: len(matches),
	}, nil
}

func (s *Service) ExtractAPIs(matches []string) []string {
	re := regexp.MustCompile(`/[\w./-]+(?:\.(?:cgi|php|action|jsp|jspx|asp|aspx|py|rb|html|htm|tpl|do|jsf))?(?:\?[\w=&%-]+)?`)
	items := make([]string, 0)
	for _, match := range matches {
		items = append(items, re.FindAllString(match, -1)...)
	}
	return removeEmptyDuplicateStrings(items)
}

func (s *Service) UnpackWxapkg(data []byte, outputDir string) ([]string, error) {
	filesInfo, err := getFilesInfo(data)
	if err != nil {
		return nil, err
	}
	files := make([]string, len(filesInfo))
	for index, fileInfo := range filesInfo {
		filename, err := safePackageOutputPath(outputDir, fileInfo.Name)
		if err != nil {
			return nil, err
		}
		files[index] = filename
	}

	writeErrors := make([]error, len(filesInfo))
	workerCount := fileWorkerCount(len(filesInfo))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				fileInfo := filesInfo[index]
				content := data[fileInfo.Offset : fileInfo.Offset+fileInfo.Size]
				if formatted, err := formatContent(files[index], content); err == nil {
					content = formatted
				}
				writeErrors[index] = writeFile(files[index], content, 0644)
			}
		}()
	}
	for index := range filesInfo {
		jobs <- index
	}
	close(jobs)
	workers.Wait()

	for _, err := range writeErrors {
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}

func (s *Service) ExtractMatches(files []string) []string {
	return flattenMatchGroups(s.ExtractMatchGroups(files))
}

func (s *Service) ExtractMatchGroups(files []string) []MatchGroup {
	groups := emptyMatchGroups(s.RuleGroups)
	groupIndexes := make(map[string]int, len(groups))
	seen := make(map[string]struct{})
	for index, group := range groups {
		groupIndexes[group.ID] = index
	}

	candidates := make([]int, 0, len(files))
	for index, file := range files {
		if _, ok := skippedExtractExts[strings.ToLower(filepath.Ext(file))]; ok {
			continue
		}
		candidates = append(candidates, index)
	}

	fileMatches := make([][]SensitiveMatch, len(files))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for worker := 0; worker < fileWorkerCount(len(candidates)); worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				data, err := os.ReadFile(files[index])
				if err == nil {
					fileMatches[index] = s.matcher.FindAllMatches(string(data))
				}
			}
		}()
	}
	for _, index := range candidates {
		jobs <- index
	}
	close(jobs)
	workers.Wait()

	for _, matches := range fileMatches {
		for _, match := range matches {
			index, exists := groupIndexes[match.GroupID]
			if !exists {
				continue
			}
			key := match.GroupID + "\x00" + match.Value
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			groups[index].Matches = append(groups[index].Matches, match)
			groups[index].Count++
		}
	}
	return groups
}

func fileWorkerCount(tasks int) int {
	if tasks <= 0 {
		return 0
	}
	workers := runtime.GOMAXPROCS(0)
	if workers > maxFileWorkers {
		workers = maxFileWorkers
	}
	if workers > tasks {
		workers = tasks
	}
	return workers
}

func emptyMatchGroups(ruleGroups []RuleGroup) []MatchGroup {
	groups := make([]MatchGroup, 0, len(ruleGroups))
	for _, group := range ruleGroups {
		if !group.Enabled {
			continue
		}
		groups = append(groups, MatchGroup{
			ID:      group.ID,
			Name:    group.Name,
			Matches: []SensitiveMatch{},
			Count:   0,
		})
	}
	return groups
}

func normalizeMatchGroups(groups []MatchGroup) []MatchGroup {
	result := make([]MatchGroup, 0, len(groups))
	for _, group := range groups {
		seen := make(map[string]struct{}, len(group.Matches))
		matches := make([]SensitiveMatch, 0, len(group.Matches))
		for _, match := range group.Matches {
			match.Value = strings.TrimSpace(match.Value)
			if match.Value == "" {
				continue
			}
			match.GroupID = group.ID
			if _, exists := seen[match.Value]; exists {
				continue
			}
			seen[match.Value] = struct{}{}
			matches = append(matches, match)
		}
		group.Matches = matches
		group.Count = len(matches)
		result = append(result, group)
	}
	return result
}

func flattenMatchGroups(groups []MatchGroup) []string {
	values := make([]string, 0)
	for _, group := range groups {
		for _, match := range group.Matches {
			values = append(values, match.Value)
		}
	}
	return removeEmptyDuplicateStrings(values)
}

func (s *Service) findWxapkgFiles(appID, version string) ([]PackageFile, error) {
	if !isSafePathSegment(appID) || !isSafePathSegment(version) {
		return nil, errors.New("appId 或 version 包含非法路径字符")
	}
	if version == rootVersionName {
		return s.findRootWxapkgFiles(appID)
	}
	dir := filepath.Join(s.resolvedAppletPath(), appID, version)
	files := make([]PackageFile, 0)
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".wxapkg") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		files = append(files, PackageFile{
			Path: path,
			Name: entry.Name(),
			Size: info.Size(),
		})
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return []PackageFile{}, nil
		}
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files, nil
}

func (s *Service) findRootWxapkgFiles(appID string) ([]PackageFile, error) {
	if !isSafePathSegment(appID) {
		return nil, errors.New("appId 包含非法路径字符")
	}
	dir := filepath.Join(s.resolvedAppletPath(), appID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []PackageFile{}, nil
		}
		return nil, err
	}

	files := make([]PackageFile, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".wxapkg") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, PackageFile{
			Path: filepath.Join(dir, entry.Name()),
			Name: entry.Name(),
			Size: info.Size(),
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files, nil
}

func (s *Service) readLocalAppInfo(appID string, versions []Version) *AppInfo {
	info := &AppInfo{
		AppID:  appID,
		Source: "local",
	}

	if avatar := s.findLocalAppAvatar(appID); avatar != "" {
		info.Avatar = avatar
	}
	if title := s.readPackageTitle(appID, versions); title != "" {
		info.Nickname = title
	}

	if info.Avatar == "" && info.Nickname == "" {
		return nil
	}
	return info
}

func (s *Service) readPackageTitle(appID string, versions []Version) string {
	for _, version := range versions {
		packages, err := s.findWxapkgFiles(appID, version.Number)
		if err != nil || len(packages) == 0 {
			continue
		}

		for _, pkg := range appTitlePackages(packages) {
			title := readWxapkgTitle(pkg.Path, appID)
			if title != "" {
				return title
			}
		}
	}
	return ""
}

func appTitlePackages(packages []PackageFile) []PackageFile {
	if len(packages) == 0 {
		return nil
	}
	for _, pkg := range packages {
		if strings.EqualFold(pkg.Name, "__APP__.wxapkg") {
			return []PackageFile{pkg}
		}
	}
	return []PackageFile{packages[0]}
}

func readWxapkgTitle(path, appID string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if shouldDecrypt(data) {
		data, err = DecryptWxapkg(data, appID, defaultSalt, defaultIV)
		if err != nil {
			return ""
		}
	}

	files, err := getFilesInfo(data)
	if err != nil {
		return ""
	}
	for _, file := range files {
		if !isAppConfigFile(file.Name) {
			continue
		}
		content := data[file.Offset : file.Offset+file.Size]
		if title := titleFromAppConfig(content); title != "" {
			return title
		}
	}
	return ""
}

func isAppConfigFile(name string) bool {
	name = strings.TrimLeft(filepath.ToSlash(strings.ToLower(name)), "/")
	return name == "app.json" || name == "app-config.json"
}

func titleFromAppConfig(content []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(content), &payload); err != nil {
		return ""
	}
	for _, keyPath := range [][]string{
		{"window", "navigationBarTitleText"},
		{"global", "window", "navigationBarTitleText"},
		{"pages", "0", "window", "navigationBarTitleText"},
	} {
		if value := stringAtJSONPath(payload, keyPath); value != "" {
			return value
		}
	}
	return ""
}

func stringAtJSONPath(value any, path []string) string {
	current := value
	for _, key := range path {
		switch typed := current.(type) {
		case map[string]any:
			current = typed[key]
		case []any:
			index := 0
			if key != "0" {
				return ""
			}
			if len(typed) <= index {
				return ""
			}
			current = typed[index]
		default:
			return ""
		}
	}
	text, _ := current.(string)
	return strings.TrimSpace(text)
}

func (s *Service) findLocalAppAvatar(appID string) string {
	for _, appDir := range s.localAppRuntimeDirs(appID) {
		candidate, ok := bestLocalAppImage(filepath.Join(appDir, "store", "images"))
		if ok {
			return "data:" + candidate.mimeType + ";base64," + base64.StdEncoding.EncodeToString(candidate.data)
		}
	}
	return ""
}

func (s *Service) localAppRuntimeDirs(appID string) []string {
	roots := s.localRuntimeRoots()
	results := make([]string, 0, len(roots))
	seen := make(map[string]struct{}, len(roots))

	for _, root := range roots {
		if root == "" {
			continue
		}
		for _, candidate := range append([]string{filepath.Join(root, appID)}, childAppRuntimeDirs(root, appID)...) {
			if !directoryExists(candidate) {
				continue
			}
			absCandidate, err := filepath.Abs(candidate)
			if err == nil {
				candidate = absCandidate
			}
			if _, ok := seen[candidate]; ok {
				continue
			}
			seen[candidate] = struct{}{}
			results = append(results, candidate)
		}
	}
	return results
}

func (s *Service) localRuntimeRoots() []string {
	appletPath := s.resolvedAppletPath()
	candidates := []string{appletPath}

	if wxRoot := findWxAppletRoot(appletPath); wxRoot != "" {
		candidates = append(candidates,
			filepath.Join(wxRoot, "Applet"),
			filepath.Join(wxRoot, "data"),
		)
	}
	if parent := filepath.Dir(appletPath); parent != "." && parent != appletPath {
		candidates = append(candidates,
			filepath.Join(parent, "Applet"),
			filepath.Join(parent, "data"),
		)
	}

	return removeDuplicatePaths(candidates)
}

func findWxAppletRoot(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	absPath, err := filepath.Abs(path)
	if err == nil {
		path = absPath
	}
	for {
		if filepath.Base(path) == ".wxapplet" {
			return path
		}
		parent := filepath.Dir(path)
		if parent == "." || parent == path {
			return ""
		}
		path = parent
	}
}

func childAppRuntimeDirs(root, appID string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	results := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(root, entry.Name(), appID)
		if directoryExists(candidate) {
			results = append(results, candidate)
		}
	}
	return results
}

func bestLocalAppImage(imageDir string) (localImageCandidate, bool) {
	entries, err := os.ReadDir(imageDir)
	if err != nil {
		return localImageCandidate{}, false
	}

	var best localImageCandidate
	bestScore := -1
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		candidate, ok := readLocalImageCandidate(filepath.Join(imageDir, entry.Name()))
		if !ok {
			continue
		}
		score := scoreLocalImageCandidate(candidate)
		if score > bestScore {
			best = candidate
			bestScore = score
		}
	}
	return best, bestScore >= 0
}

func readLocalImageCandidate(path string) (localImageCandidate, bool) {
	info, err := os.Stat(path)
	if err != nil || info.Size() <= 0 || info.Size() > 512*1024 {
		return localImageCandidate{}, false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return localImageCandidate{}, false
	}
	mimeType := http.DetectContentType(data)
	if mimeType != "image/png" && mimeType != "image/jpeg" && mimeType != "image/gif" {
		return localImageCandidate{}, false
	}

	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return localImageCandidate{}, false
	}
	if config.Width < 32 || config.Height < 32 || config.Width > 256 || config.Height > 256 {
		return localImageCandidate{}, false
	}

	return localImageCandidate{
		data:     data,
		mimeType: mimeType,
		width:    config.Width,
		height:   config.Height,
		size:     info.Size(),
	}, true
}

func scoreLocalImageCandidate(candidate localImageCandidate) int {
	score := 0
	diff := candidate.width - candidate.height
	if diff < 0 {
		diff = -diff
	}
	if diff <= 2 {
		score += 1000
	}
	if candidate.width >= 64 && candidate.width <= 180 {
		score += 500
	}
	if candidate.width >= 96 && candidate.width <= 160 {
		score += 200
	}
	score += candidate.width
	if candidate.size < 24*1024 {
		score += 50
	}
	return score
}

func (s *Service) resolvedAppletPath() string {
	return ResolveAppletPath(s.AppletPath)
}

func (s *Service) versionOutputDir(appID, version string) string {
	return filepath.Join(s.OutputRoot, appID, version)
}

func resolvePackageOutputDir(outputDir, relPath, packageName string) string {
	if strings.EqualFold(packageName, "__APP__.wxapkg") {
		return outputDir
	}
	return filepath.Join(outputDir, strings.TrimSuffix(relPath, filepath.Ext(relPath)))
}

func newestPackageModTime(packages []PackageFile, fallback time.Time) time.Time {
	newest := fallback
	for _, pkg := range packages {
		info, err := os.Stat(pkg.Path)
		if err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return newest
}

func DecryptWxapkg(data []byte, wxID, salt, iv string) ([]byte, error) {
	if len(data) < 1030 {
		return nil, errors.New("wxapkg 文件过小，无法解密")
	}
	if len(iv) != aes.BlockSize {
		return nil, errors.New("IV 长度必须为 16 字节")
	}

	dk := pbkdf2.Key([]byte(wxID), []byte(salt), 1000, 32, sha1.New)
	block, err := aes.NewCipher(dk)
	if err != nil {
		return nil, err
	}
	blockMode := cipher.NewCBCDecrypter(block, []byte(iv))
	originData := make([]byte, 1024)
	blockMode.CryptBlocks(originData, data[6:1030])

	afData := make([]byte, len(data)-1030)
	xorKey := byte(0x66)
	if len(wxID) >= 2 {
		xorKey = wxID[len(wxID)-2]
	}
	for i, b := range data[1030:] {
		afData[i] = b ^ xorKey
	}
	return append(originData[:1023], afData...), nil
}

func getFilesInfo(data []byte) ([]wxapkgFile, error) {
	reader := bytes.NewReader(data)

	var firstMark byte
	if err := readBinary(reader, &firstMark); err != nil {
		return nil, fmt.Errorf("读取首标记失败: %w", err)
	}
	if firstMark != 0xBE {
		return nil, errors.New("无效的 wxapkg 文件: 首标记不正确")
	}

	var info1, indexInfoLength, bodyInfoLength uint32
	if err := readBinary(reader, &info1); err != nil {
		return nil, fmt.Errorf("读取 info1 失败: %w", err)
	}
	if err := readBinary(reader, &indexInfoLength); err != nil {
		return nil, fmt.Errorf("读取索引段长度失败: %w", err)
	}
	if err := readBinary(reader, &bodyInfoLength); err != nil {
		return nil, fmt.Errorf("读取数据段长度失败: %w", err)
	}

	if uint64(indexInfoLength)+uint64(bodyInfoLength) > uint64(len(data)) {
		return nil, fmt.Errorf("文件长度不足, 索引段(%d) + 数据段(%d) > 文件总长度(%d)", indexInfoLength, bodyInfoLength, len(data))
	}
	totalLength := uint64(len(data))

	var lastMark byte
	if err := readBinary(reader, &lastMark); err != nil {
		return nil, fmt.Errorf("读取尾标记失败: %w", err)
	}
	if lastMark != 0xED {
		return nil, errors.New("无效的 wxapkg 文件: 尾标记不正确")
	}

	var fileCount uint32
	if err := readBinary(reader, &fileCount); err != nil {
		return nil, fmt.Errorf("读取文件数量失败: %w", err)
	}

	expectedIndexEnd := uint64(reader.Size()) - uint64(bodyInfoLength)
	fileList := make([]wxapkgFile, fileCount)
	for i := range fileList {
		name, err := readFileName(reader)
		if err != nil {
			return nil, err
		}

		item := wxapkgFile{Name: name}
		if err := readBinary(reader, &item.Offset); err != nil {
			return nil, fmt.Errorf("读取文件偏移量失败: %w", err)
		}
		if err := readBinary(reader, &item.Size); err != nil {
			return nil, fmt.Errorf("读取文件大小失败: %w", err)
		}

		fileEnd := uint64(item.Offset) + uint64(item.Size)
		if fileEnd > totalLength {
			return nil, fmt.Errorf("文件 %s 的结束位置 (%d) 超出了文件总长度 (%d)", item.Name, fileEnd, totalLength)
		}

		currentPos := uint64(reader.Size()) - uint64(reader.Len())
		if currentPos > expectedIndexEnd {
			return nil, fmt.Errorf("索引读取超出预期范围: 当前位置 %d, 预期索引结束位置 %d", currentPos, expectedIndexEnd)
		}
		fileList[i] = item
	}

	currentPos := uint64(reader.Size()) - uint64(reader.Len())
	if currentPos != expectedIndexEnd {
		return nil, fmt.Errorf("索引段长度不符: 读取到位置 %d, 预期结束位置 %d", currentPos, expectedIndexEnd)
	}
	return fileList, nil
}

func readFileName(reader *bytes.Reader) (string, error) {
	var nameLen uint32
	if err := readBinary(reader, &nameLen); err != nil {
		return "", fmt.Errorf("读取文件名长度失败: %w", err)
	}
	if nameLen == 0 || nameLen > 4096 {
		return "", fmt.Errorf("文件名长度 %d 不合理", nameLen)
	}

	nameBytes := make([]byte, nameLen)
	if _, err := io.ReadAtLeast(reader, nameBytes, int(nameLen)); err != nil {
		return "", fmt.Errorf("读取文件名失败: %w", err)
	}
	return string(nameBytes), nil
}

func shouldDecrypt(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	return data[0] != 0xBE
}

func readBinary(reader *bytes.Reader, data any) error {
	return binary.Read(reader, binary.BigEndian, data)
}

func formatContent(filename string, content []byte) ([]byte, error) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".js":
		// jsbeautifier builds its output through repeated string concatenation.
		// Large minified bundles can therefore take minutes to format and block
		// the whole decompile request. Keep their original source instead; it is
		// still extracted and scanned just like formatted JavaScript.
		if len(content) > maxJSBeautifyBytes {
			return content, nil
		}
		code := string(bytes.TrimSpace(content))
		formatted, err := jsbeautifier.Beautify(&code, jsbeautifier.DefaultOptions())
		if err != nil {
			return content, err
		}
		return []byte(formatted), nil
	case ".json":
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, bytes.TrimSpace(content), "", "    "); err != nil {
			return content, err
		}
		return pretty.Bytes(), nil
	case ".html":
		return gohtml.FormatBytes(bytes.TrimSpace(content)), nil
	default:
		return content, nil
	}
}

func safePackageOutputPath(root, packageName string) (string, error) {
	name := filepath.FromSlash(strings.TrimLeft(packageName, `/\`))
	clean := filepath.Clean(name)
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." || filepath.IsAbs(clean) {
		return "", fmt.Errorf("非法包内文件路径: %s", packageName)
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target := filepath.Join(rootAbs, clean)
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if targetAbs != rootAbs && !strings.HasPrefix(targetAbs, rootAbs+string(filepath.Separator)) {
		return "", fmt.Errorf("非法包内文件路径: %s", packageName)
	}
	return targetAbs, nil
}

func writeFile(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}

func writeResultFiles(outputDir string, result *DecompileResult) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outputDir, matchesFileName), []byte(strings.Join(result.Matched, "\n")), 0644); err != nil {
		return err
	}
	groupedMatches, err := json.MarshalIndent(GroupedMatchesResult{
		Matches: result.Matched,
		Groups:  result.MatchGroups,
		Total:   result.MatchedCount,
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outputDir, groupedMatchesFileName), groupedMatches, 0644); err != nil {
		return err
	}
	manifest, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outputDir, manifestFileName), manifest, 0644)
}

func sampleRelativeFiles(root string, files []string, limit int) []string {
	if limit <= 0 || len(files) == 0 {
		return []string{}
	}
	result := make([]string, 0, minInt(limit, len(files)))
	for _, file := range files {
		if len(result) >= limit {
			break
		}
		rel, err := filepath.Rel(root, file)
		if err != nil {
			result = append(result, file)
			continue
		}
		result = append(result, filepath.ToSlash(rel))
	}
	return result
}

func directoryHasFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 0
}

func directoryExists(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

func formatModTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

func defaultAppletPathCandidates(homeDir string) []string {
	candidates := []string{}
	for _, key := range []string{"WECHAT_APPLET_PATH", "WXAPPLET_PATH"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			candidates = append(candidates, value)
		}
	}

	switch runtime.GOOS {
	case "darwin":
		wxAppletRoot := filepath.Join(homeDir, "Library", "Containers", "com.tencent.xinWeChat", "Data", ".wxapplet")
		candidates = append(candidates,
			filepath.Join(wxAppletRoot, "packages"),
			wxAppletRoot,
			filepath.Join(wxAppletRoot, "Applet"),
			filepath.Join(wxAppletRoot, "WMPF", "applet"),
			filepath.Join(homeDir, "Library", "Containers", "com.tencent.xinWeChat", "Data", "Library", "Application Support", "com.tencent.xinWeChat"),
			filepath.Join(homeDir, "Library", "Application Support", "com.tencent.xinWeChat"),
		)
	case "windows":
		documentsWeChat := filepath.Join(homeDir, "Documents", "WeChat Files")
		candidates = append(candidates,
			filepath.Join(documentsWeChat, "Applet"),
			documentsWeChat,
		)
		for _, envKey := range []string{"APPDATA", "LOCALAPPDATA"} {
			if dir := strings.TrimSpace(os.Getenv(envKey)); dir != "" {
				candidates = append(candidates,
					filepath.Join(dir, "Tencent", "WeChat", "Applet"),
					filepath.Join(dir, "WeChat", "Applet"),
				)
			}
		}
	default:
		candidates = append(candidates,
			filepath.Join(homeDir, ".wxapplet", "packages"),
			filepath.Join(homeDir, ".wxapplet"),
			filepath.Join(homeDir, "Documents", "WeChat Files", "Applet"),
		)
	}
	return candidates
}

func appletPathCandidates(root string) []string {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}

	candidates := []string{
		root,
		filepath.Join(root, "packages"),
		filepath.Join(root, "Applet"),
		filepath.Join(root, "applet"),
		filepath.Join(root, "WMPF", "applet"),
	}
	if parent := filepath.Dir(root); parent != "." && parent != root {
		candidates = append(candidates, filepath.Join(parent, "packages"))
	}

	expanded := make([]string, 0, len(candidates)*2)
	for _, candidate := range candidates {
		expanded = append(expanded, candidate)
		expanded = append(expanded, immediateChildDirs(candidate)...)
	}
	return expanded
}

func expandAppletPathCandidates(candidates []string) []string {
	expanded := make([]string, 0, len(candidates)*2)
	for _, candidate := range candidates {
		expanded = append(expanded, appletPathCandidates(candidate)...)
	}
	return expanded
}

func bestAppletPathCandidate(candidates []string) string {
	seen := make(map[string]struct{}, len(candidates))
	bestPath := ""
	bestScore := 0
	var bestModTime time.Time
	firstExisting := ""

	for _, candidate := range candidates {
		candidate = normalizeAppletPathCandidate(candidate)
		if candidate == "" {
			continue
		}
		absCandidate, err := filepath.Abs(candidate)
		if err == nil {
			candidate = absCandidate
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}

		score, modTime, exists := scoreAppletPath(candidate)
		if !exists {
			continue
		}
		if firstExisting == "" {
			firstExisting = candidate
		}
		if score > bestScore || (score == bestScore && score > 0 && modTime.After(bestModTime)) {
			bestPath = candidate
			bestScore = score
			bestModTime = modTime
		}
	}

	if bestPath != "" {
		return bestPath
	}
	return firstExisting
}

func scoreAppletPath(root string) (int, time.Time, bool) {
	if isPluginAppletPath(root) {
		return 0, time.Time{}, false
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return 0, time.Time{}, false
	}

	wxDirCount := 0
	packageVersionCount := 0
	var newest time.Time
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "wx") {
			continue
		}

		wxDirCount++
		appDir := filepath.Join(root, entry.Name())
		if info, err := entry.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		if hasDirectWxapkg(appDir) {
			packageVersionCount++
		}

		versionEntries, err := os.ReadDir(appDir)
		if err != nil {
			continue
		}
		for _, versionEntry := range versionEntries {
			if !versionEntry.IsDir() {
				continue
			}
			versionDir := filepath.Join(appDir, versionEntry.Name())
			if hasDirectWxapkg(versionDir) {
				packageVersionCount++
				if info, err := versionEntry.Info(); err == nil && info.ModTime().After(newest) {
					newest = info.ModTime()
				}
			}
		}
	}

	return packageVersionCount*1000 + wxDirCount*10, newest, true
}

func normalizeAppletPathCandidate(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	if absPath, err := filepath.Abs(clean); err == nil {
		clean = absPath
	}

	current := clean
	for {
		if filepath.Base(current) == pluginDirName {
			return filepath.Dir(current)
		}
		parent := filepath.Dir(current)
		if parent == "." || parent == current {
			return clean
		}
		current = parent
	}
}

func isPluginAppletPath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	current := filepath.Clean(path)
	for {
		if filepath.Base(current) == pluginDirName {
			return true
		}
		parent := filepath.Dir(current)
		if parent == "." || parent == current {
			return false
		}
		current = parent
	}
}

func hasDirectWxapkg(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".wxapkg") {
			return true
		}
	}
	return false
}

func immediateChildDirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	children := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != pluginDirName {
			children = append(children, filepath.Join(dir, entry.Name()))
		}
	}
	return children
}

func isSafePathSegment(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && value != "." && value != ".." && !strings.ContainsAny(value, `/\`)
}

func sortDirEntriesByModTimeDesc(entries []os.DirEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		left, leftErr := entries[i].Info()
		right, rightErr := entries[j].Info()
		if leftErr != nil || rightErr != nil {
			return entries[i].Name() < entries[j].Name()
		}
		return left.ModTime().After(right.ModTime())
	})
}

func removeEmptyDuplicateStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func removeDuplicatePaths(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if absItem, err := filepath.Abs(item); err == nil {
			item = absItem
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type Matcher struct {
	rules   []matcherRule
	regexes map[string]*regexp.Regexp
}

type matcherRule struct {
	groupID string
	ruleID  string
	pattern string
}

func NewMatcher(rules []string) *Matcher {
	return NewGroupedMatcher(RuleGroupsFromLegacyRules(rules))
}

func NewGroupedMatcher(groups []RuleGroup) *Matcher {
	rules := make([]matcherRule, 0)
	regexes := make(map[string]*regexp.Regexp)
	for _, group := range NormalizeRuleGroups(groups) {
		if !group.Enabled {
			continue
		}
		for _, rule := range group.Rules {
			if !rule.Enabled {
				continue
			}
			rules = append(rules, matcherRule{
				groupID: group.ID,
				ruleID:  rule.ID,
				pattern: rule.Pattern,
			})
			if _, exists := regexes[rule.Pattern]; !exists {
				if re, err := regexp.Compile(rule.Pattern); err == nil {
					regexes[rule.Pattern] = re
				}
			}
		}
	}
	return &Matcher{
		rules:   rules,
		regexes: regexes,
	}
}

func (m *Matcher) FindAll(content string) []string {
	values := make([]string, 0)
	for _, match := range m.FindAllMatches(content) {
		values = append(values, match.Value)
	}
	return removeEmptyDuplicateStrings(values)
}

func (m *Matcher) FindAllMatches(content string) []SensitiveMatch {
	matches := make([]SensitiveMatch, 0)
	seen := make(map[string]struct{})
	for _, rule := range m.rules {
		re, err := m.getRegex(rule.pattern)
		if err != nil {
			continue
		}
		for _, value := range re.FindAllString(content, -1) {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			key := rule.groupID + "\x00" + value
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			matches = append(matches, SensitiveMatch{
				Value:   value,
				GroupID: rule.groupID,
				RuleID:  rule.ruleID,
			})
		}
	}
	return matches
}

func (m *Matcher) getRegex(rule string) (*regexp.Regexp, error) {
	if re, ok := m.regexes[rule]; ok {
		return re, nil
	}
	return nil, fmt.Errorf("无效的正则表达式: %s", rule)
}
