package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	aipkg "hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	mcpWorkbenchPythonMaxCodeBytes   = 200000
	mcpWorkbenchPythonDefaultTimeout = 10
	mcpWorkbenchPythonMaxTimeout     = 30
	mcpWorkbenchPythonOutputLimit    = 20000
)

type mcpWorkbenchPythonExecuteArgs struct {
	TaskID         string                 `json:"taskId"`
	StepID         string                 `json:"stepId"`
	Code           string                 `json:"code"`
	TimeoutSeconds int                    `json:"timeoutSeconds"`
	RiskLevel      string                 `json:"riskLevel"`
	ApprovalID     string                 `json:"approvalId"`
	Inputs         map[string]interface{} `json:"inputs"`
}

type mcpWorkbenchPythonRunnerRequest struct {
	Code    string                 `json:"code"`
	WorkDir string                 `json:"workDir"`
	Scope   []string               `json:"scope"`
	Inputs  map[string]interface{} `json:"inputs"`
}

type mcpWorkbenchPythonRunnerResult struct {
	OK      bool                   `json:"ok"`
	Stdout  string                 `json:"stdout"`
	Stderr  string                 `json:"stderr"`
	Result  string                 `json:"result"`
	Files   []string               `json:"files"`
	Error   string                 `json:"error"`
	Details map[string]interface{} `json:"details"`
}

func mcpCallWorkbenchPythonExecute(args mcpWorkbenchPythonExecuteArgs) (mcpToolCallResult, error) {
	db, task, err := loadMCPWorkbenchTask(args.TaskID)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	code := strings.TrimSpace(args.Code)
	if code == "" {
		return mcpToolCallResult{}, fmt.Errorf("code 不能为空")
	}
	if len(code) > mcpWorkbenchPythonMaxCodeBytes {
		return mcpToolCallResult{}, fmt.Errorf("code 超过最大长度 %d bytes", mcpWorkbenchPythonMaxCodeBytes)
	}
	riskLevel := normalizeMCPWorkbenchRiskLevel(args.RiskLevel)
	if riskLevel == "high" || riskLevel == "critical" {
		if err := ensureMCPWorkbenchApprovalApproved(db, task.ID, args.ApprovalID); err != nil {
			return mcpToolCallResult{}, err
		}
	}
	pythonPath, err := exec.LookPath("python3")
	if err != nil {
		return mcpToolCallResult{}, fmt.Errorf("未找到 python3: %w", err)
	}
	timeout := args.TimeoutSeconds
	if timeout <= 0 {
		timeout = mcpWorkbenchPythonDefaultTimeout
	}
	if timeout > mcpWorkbenchPythonMaxTimeout {
		timeout = mcpWorkbenchPythonMaxTimeout
	}
	executionID := database.GenerateNanoid()
	workDir := filepath.Join(config.GetDataDir(), "ai-workbench", "tasks", sanitizeMCPWorkbenchPathSegment(task.ID), "python", executionID)
	if err := os.MkdirAll(workDir, 0750); err != nil {
		return mcpToolCallResult{}, fmt.Errorf("创建 Python 工作目录失败: %w", err)
	}
	runnerPath := filepath.Join(workDir, "runner.py")
	if err := os.WriteFile(runnerPath, []byte(mcpWorkbenchPythonRunnerSource), 0600); err != nil {
		return mcpToolCallResult{}, fmt.Errorf("写入 Python runner 失败: %w", err)
	}
	request := mcpWorkbenchPythonRunnerRequest{
		Code:    code,
		WorkDir: workDir,
		Scope:   append([]string{task.Target}, task.Scope...),
		Inputs:  args.Inputs,
	}
	requestJSON, _ := json.Marshal(request)
	toolCallID := database.GenerateNanoid()
	toolCall := database.AIWorkbenchToolCall{
		ID:          toolCallID,
		TaskID:      task.ID,
		StepID:      strings.TrimSpace(args.StepID),
		ExecutionID: executionID,
		ToolName:    "python_execute",
		InputJSON: marshalMCPWorkbenchJSON(map[string]interface{}{
			"codePreview":    truncateMCPWorkbenchText(code, 1000),
			"timeoutSeconds": timeout,
			"riskLevel":      riskLevel,
			"approvalId":     strings.TrimSpace(args.ApprovalID),
			"inputs":         args.Inputs,
		}),
		Status:    "running",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&toolCall).Error; err != nil {
			return err
		}
		return createMCPWorkbenchToolExecution(tx, database.AIWorkbenchToolExecution{
			ID:           executionID,
			TaskID:       task.ID,
			StepID:       toolCall.StepID,
			ToolCallID:   toolCall.ID,
			ToolName:     "python.execute",
			Runtime:      aipkg.WorkbenchToolRuntimeLocalMCP,
			InputSummary: fmt.Sprintf("Python 分析脚本（%d bytes，超时 %d 秒）", len(code), timeout),
			Arguments: map[string]any{
				"codePreview":    truncateMCPWorkbenchText(code, 1000),
				"timeoutSeconds": timeout,
				"riskLevel":      riskLevel,
				"approvalId":     strings.TrimSpace(args.ApprovalID),
				"inputs":         args.Inputs,
			},
			Status: aipkg.WorkbenchToolExecutionStatusRunning,
			Metadata: map[string]any{
				"toolCallId": toolCall.ID,
				"workDir":    workDir,
			},
		})
	}); err != nil {
		return mcpToolCallResult{}, err
	}
	result, runErr := runMCPWorkbenchPythonRunner(pythonPath, runnerPath, requestJSON, timeout)
	result.Stdout = truncateMCPWorkbenchText(result.Stdout, mcpWorkbenchPythonOutputLimit)
	result.Stderr = truncateMCPWorkbenchText(result.Stderr, mcpWorkbenchPythonOutputLimit)
	result.Result = truncateMCPWorkbenchText(result.Result, mcpWorkbenchPythonOutputLimit)
	result.Error = truncateMCPWorkbenchText(result.Error, mcpWorkbenchPythonOutputLimit)
	resultPath := filepath.Join(workDir, "result.json")
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	_ = os.WriteFile(resultPath, resultJSON, 0600)

	summary := buildMCPWorkbenchPythonSummary(result, runErr)
	if err := db.Transaction(func(tx *gorm.DB) error {
		toolCall.Status = "completed"
		if runErr != nil || !result.OK {
			toolCall.Status = "failed"
			toolCall.ErrorMessage = defaultString(result.Error, runErrString(runErr))
		}
		toolCall.OutputSummary = summary
		toolCall.RawOutputRef = resultPath
		toolCall.UpdatedAt = time.Now()
		if err := tx.Save(&toolCall).Error; err != nil {
			return err
		}
		if err := finishMCPWorkbenchToolExecution(tx, executionID, toolCall.Status, summary, resultPath, toolCall.ErrorMessage, map[string]any{
			"toolCallId": toolCall.ID,
			"workDir":    workDir,
			"resultPath": resultPath,
			"files":      result.Files,
		}); err != nil {
			return err
		}
		eventType := "python.completed"
		level := "info"
		title := "Python 分析执行完成"
		if toolCall.Status == "failed" {
			eventType = "python.failed"
			level = "error"
			title = "Python 分析执行失败"
		}
		if err := createMCPWorkbenchEvent(tx, task.ID, toolCall.StepID, eventType, level, title, summary, gin.H{
			"toolCallId": toolCall.ID,
			"workDir":    workDir,
			"resultPath": resultPath,
			"files":      result.Files,
		}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return mcpToolCallResult{}, err
	}
	if result.OK {
		_ = mcpCallWorkbenchFactUpsertNoResult(mcpWorkbenchFactUpsertArgs{
			TaskID:     task.ID,
			StepID:     args.StepID,
			FactKey:    "python/" + executionID,
			Source:     "python_execute",
			TargetRef:  workDir,
			Summary:    summary,
			Body:       strings.TrimSpace(result.Stdout + "\n" + result.Result),
			Confidence: "medium",
			Metadata: map[string]interface{}{
				"toolCallId": toolCall.ID,
				"resultPath": resultPath,
				"files":      result.Files,
			},
		})
		_ = mcpCallWorkbenchEvidenceAddNoResult(mcpWorkbenchEvidenceAddArgs{
			TaskID:     task.ID,
			StepID:     args.StepID,
			SourceType: "python_execute",
			SourceID:   toolCall.ID,
			TargetRef:  workDir,
			Title:      "Python 高级分析结果",
			Summary:    summary,
			RiskLevel:  "info",
			Confidence: "medium",
			Reference:  resultPath,
			Metadata: map[string]interface{}{
				"stdout": truncateMCPWorkbenchText(result.Stdout, 2000),
				"result": truncateMCPWorkbenchText(result.Result, 2000),
				"files":  result.Files,
			},
		})
	}
	if runErr != nil {
		return mcpToolCallResult{}, runErr
	}
	if !result.OK {
		return mcpToolCallResult{}, errors.New(result.Error)
	}
	return newMCPToolResult(gin.H{
		"taskId":      task.ID,
		"executionId": executionID,
		"toolCallId":  toolCall.ID,
		"workDir":     workDir,
		"resultPath":  resultPath,
		"stdout":      result.Stdout,
		"stderr":      result.Stderr,
		"result":      result.Result,
		"files":       result.Files,
	}), nil
}

func runMCPWorkbenchPythonRunner(pythonPath string, runnerPath string, requestJSON []byte, timeoutSeconds int) (mcpWorkbenchPythonRunnerResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, pythonPath, "-I", "-S", runnerPath)
	configureMCPWorkbenchCommand(cmd)
	cmd.Stdin = bytes.NewReader(requestJSON)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return mcpWorkbenchPythonRunnerResult{OK: false, Error: "Python 执行超时", Stderr: stderr.String()}, ctx.Err()
	}
	var result mcpWorkbenchPythonRunnerResult
	if stdout.Len() > 0 {
		if jsonErr := json.Unmarshal(stdout.Bytes(), &result); jsonErr != nil {
			return mcpWorkbenchPythonRunnerResult{OK: false, Stdout: stdout.String(), Stderr: stderr.String(), Error: jsonErr.Error()}, jsonErr
		}
	}
	if result.Stderr == "" {
		result.Stderr = stderr.String()
	}
	if err != nil {
		if result.Error == "" {
			result.Error = err.Error()
		}
		return result, err
	}
	return result, nil
}

func ensureMCPWorkbenchApprovalApproved(db *gorm.DB, taskID string, approvalID string) error {
	approvalID = strings.TrimSpace(approvalID)
	if approvalID == "" {
		return fmt.Errorf("高风险 Python 执行需要 approvalId")
	}
	var approval database.AIWorkbenchApproval
	if err := db.Where("id = ? AND task_id = ? AND status = ?", approvalID, taskID, "approved").First(&approval).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("高风险 Python 执行未找到已批准审批: %s", approvalID)
		}
		return err
	}
	return nil
}

func buildMCPWorkbenchPythonSummary(result mcpWorkbenchPythonRunnerResult, runErr error) string {
	if runErr != nil {
		return "Python 执行失败: " + runErr.Error()
	}
	if !result.OK {
		return "Python 执行失败: " + result.Error
	}
	parts := []string{"Python 执行完成"}
	if strings.TrimSpace(result.Result) != "" {
		parts = append(parts, "result="+truncateMCPWorkbenchText(singleLineMCPWorkbenchText(result.Result), 240))
	}
	if strings.TrimSpace(result.Stdout) != "" {
		parts = append(parts, "stdout="+truncateMCPWorkbenchText(singleLineMCPWorkbenchText(result.Stdout), 240))
	}
	if len(result.Files) > 0 {
		parts = append(parts, fmt.Sprintf("files=%d", len(result.Files)))
	}
	return strings.Join(parts, "；")
}

func runErrString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func singleLineMCPWorkbenchText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func sanitizeMCPWorkbenchPathSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	builder := strings.Builder{}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			builder.WriteRune(r)
		}
	}
	if builder.Len() == 0 {
		return "-"
	}
	return builder.String()
}

const mcpWorkbenchPythonRunnerSource = `
import ast
import builtins
import contextlib
import io
import ipaddress
import json
import pathlib
import re
import sys
import traceback
import urllib.error
import urllib.parse
import urllib.request

DENIED_IMPORTS = {"os", "subprocess", "socket", "ctypes", "multiprocessing", "threading", "asyncio", "shutil", "pathlib", "importlib", "sys", "builtins"}
ALLOWED_IMPORTS = {"json", "re", "base64", "hashlib", "hmac", "math", "statistics", "collections", "datetime", "urllib.parse"}
DENIED_CALLS = {"eval", "exec", "compile", "open", "__import__", "input", "breakpoint", "help", "dir", "globals", "locals", "vars"}

def fail(message, details=None):
    print(json.dumps({"ok": False, "error": message, "details": details or {}}, ensure_ascii=False))
    sys.exit(0)

def validate_ast(code):
    try:
        tree = ast.parse(code, mode="exec")
    except SyntaxError as exc:
        fail("Python 语法错误", {"line": exc.lineno, "message": exc.msg})
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            for alias in node.names:
                root = alias.name.split(".")[0]
                if root in DENIED_IMPORTS or alias.name not in ALLOWED_IMPORTS and root not in ALLOWED_IMPORTS:
                    fail("Python import 不在允许范围内", {"module": alias.name})
        if isinstance(node, ast.ImportFrom):
            root = (node.module or "").split(".")[0]
            if root in DENIED_IMPORTS or (node.module or "") not in ALLOWED_IMPORTS and root not in ALLOWED_IMPORTS:
                fail("Python import 不在允许范围内", {"module": node.module})
        if isinstance(node, ast.Call):
            name = ""
            if isinstance(node.func, ast.Name):
                name = node.func.id
            elif isinstance(node.func, ast.Attribute):
                name = node.func.attr
            if name in DENIED_CALLS:
                fail("Python 调用不在允许范围内", {"call": name})
    return tree

def normalize_host(value):
    parsed = urllib.parse.urlparse(value)
    if parsed.hostname:
        return parsed.hostname.lower()
    return value.lower()

def in_scope(target, scope):
    parsed = urllib.parse.urlparse(target)
    if parsed.scheme not in ("http", "https") or not parsed.hostname:
        return False
    target_host = parsed.hostname.lower()
    normalized_target = target.rstrip("/").lower()
    for item in scope:
        item = str(item or "").strip()
        if not item:
            continue
        if normalized_target == item.rstrip("/").lower():
            return True
        scope_host = normalize_host(item)
        if target_host == scope_host or target_host.endswith("." + scope_host):
            return True
        try:
            if ipaddress.ip_address(target_host) in ipaddress.ip_network(item, strict=False):
                return True
        except Exception:
            pass
    return False

def make_safe_open(work_dir):
    root = pathlib.Path(work_dir).resolve()
    def safe_path(name):
        candidate = (root / str(name)).resolve()
        if root != candidate and root not in candidate.parents:
            raise PermissionError("只能访问任务 Python 工作目录")
        return candidate
    def read_text(name, encoding="utf-8"):
        return safe_path(name).read_text(encoding=encoding)
    def write_text(name, content, encoding="utf-8"):
        path = safe_path(name)
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(str(content), encoding=encoding)
        return str(path.relative_to(root))
    return read_text, write_text

def scoped_http_request(scope, method, url, headers=None, body=None, timeout=10):
    method = str(method or "GET").upper()
    if method not in {"GET", "HEAD", "OPTIONS", "POST", "PUT", "PATCH"}:
        raise PermissionError("HTTP method 未开放")
    if not in_scope(str(url), scope):
        raise PermissionError("URL 不在任务授权范围内")
    data = None
    if body is not None:
        data = str(body).encode("utf-8")
    req = urllib.request.Request(str(url), data=data, method=method, headers=headers or {})
    with urllib.request.urlopen(req, timeout=min(int(timeout or 10), 30)) as resp:
        raw = resp.read(512 * 1024 + 1)
        text = raw[:512 * 1024].decode("utf-8", errors="replace")
        if len(raw) > 512 * 1024:
            text += "\n[truncated]"
        return {
            "status": resp.status,
            "headers": dict(resp.headers.items()),
            "body": text,
            "length": len(raw),
        }

def safe_import(name, globals=None, locals=None, fromlist=(), level=0):
    root = name.split(".")[0]
    if root in ALLOWED_IMPORTS or name in ALLOWED_IMPORTS:
        return builtins.__import__(name, globals, locals, fromlist, level)
    raise ImportError("模块不在允许范围内: " + name)

def main():
    request = json.loads(sys.stdin.read())
    code = request.get("code") or ""
    work_dir = request.get("workDir") or "."
    scope = request.get("scope") or []
    inputs = request.get("inputs") or {}
    tree = validate_ast(code)
    read_text, write_text = make_safe_open(work_dir)
    stdout = io.StringIO()
    stderr = io.StringIO()
    safe_builtins = {
        "__import__": safe_import,
        "abs": abs,
        "all": all,
        "any": any,
        "bool": bool,
        "dict": dict,
        "enumerate": enumerate,
        "Exception": Exception,
        "float": float,
        "int": int,
        "isinstance": isinstance,
        "len": len,
        "list": list,
        "map": map,
        "max": max,
        "min": min,
        "print": print,
        "range": range,
        "repr": repr,
        "round": round,
        "set": set,
        "sorted": sorted,
        "str": str,
        "sum": sum,
        "tuple": tuple,
        "ValueError": ValueError,
        "zip": zip,
    }
    env = {
        "__builtins__": safe_builtins,
        "inputs": inputs,
        "read_text": read_text,
        "write_text": write_text,
        "scoped_http_request": lambda method, url, headers=None, body=None, timeout=10: scoped_http_request(scope, method, url, headers, body, timeout),
    }
    try:
        with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
            exec(compile(tree, "<hephaestus-python-runner>", "exec"), env, env)
        result = env.get("_result", "")
        if not isinstance(result, str):
            result = json.dumps(result, ensure_ascii=False, default=str)
        root = pathlib.Path(work_dir).resolve()
        files = []
        for item in root.rglob("*"):
            if item.is_file() and item.name not in {"runner.py", "result.json"}:
                files.append(str(item.relative_to(root)))
        print(json.dumps({
            "ok": True,
            "stdout": stdout.getvalue(),
            "stderr": stderr.getvalue(),
            "result": result,
            "files": files[:50],
        }, ensure_ascii=False))
    except Exception as exc:
        print(json.dumps({
            "ok": False,
            "stdout": stdout.getvalue(),
            "stderr": stderr.getvalue(),
            "error": str(exc),
            "details": {"traceback": traceback.format_exc(limit=6)},
        }, ensure_ascii=False))

main()
`
