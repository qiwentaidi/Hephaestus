package logger

import (
	"fmt"
	"hephaestus-backend/pkg/config"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	instance       *Logger
	once           sync.Once
	originalStdout = os.Stdout
)

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// Logger 日志记录器
type Logger struct {
	mu       sync.Mutex
	file     *os.File
	filePath string
	date     string
	writer   io.Writer
}

type fileFirstWriter struct {
	file    io.Writer
	console io.Writer
}

func (w fileFirstWriter) Write(p []byte) (int, error) {
	n, err := w.file.Write(p)
	if w.console != nil {
		_, _ = w.console.Write(p)
	}
	return n, err
}

// LogLevel 日志级别
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARNING
	ERROR
	VULN
)

var levelNames = map[LogLevel]string{
	DEBUG:   "DEBUG",
	INFO:    "INFO",
	WARNING: "WARNING",
	ERROR:   "ERROR",
	VULN:    "VULN",
}

// GetLogger 获取全局日志实例
func GetLogger() *Logger {
	once.Do(func() {
		instance = NewLogger()
	})
	return instance
}

// NewLogger 创建新的日志记录器
func NewLogger() *Logger {
	logger := &Logger{}
	logger.init()
	return logger
}

// init 初始化日志记录器
func (l *Logger) init() {
	logsDir := config.GetLogsDir()
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		fmt.Printf("创建日志目录失败: %v\n", err)
		return
	}

	cleanupExpiredLogs(logsDir, time.Now())

	// 打开今天的日志文件
	l.openTodayLog(logsDir)
	l.refreshConsoleWriter()
}

// openTodayLog 打开今天的日志文件
func (l *Logger) openTodayLog(logsDir string) {
	today := time.Now().Format("2006-01-02")

	// 如果日期没变，不需要重新打开
	if l.date == today && l.file != nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// 关闭旧文件
	if l.file != nil {
		l.file.Close()
		l.file = nil
		l.writer = nil
	}

	cleanupExpiredLogs(logsDir, time.Now())

	// 打开新文件
	fileName := fmt.Sprintf("%s.log", today)
	l.filePath = filepath.Join(logsDir, fileName)

	file, err := os.OpenFile(l.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Printf("打开日志文件失败: %v\n", err)
		return
	}

	l.file = file
	l.date = today
	// Windows 桌面 GUI 进程可能没有有效 stdout。文件写入必须优先完成，
	// 控制台输出只作为 best-effort，避免 stdout 失败导致日志文件保持 0 字节。
	l.writer = fileFirstWriter{file: file, console: originalStdout}
}

func (l *Logger) refreshConsoleWriter() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		l.writer = fileFirstWriter{file: l.file, console: originalStdout}
	}
}

// checkAndRotate 检查并轮转日志文件（如果日期变化）
func (l *Logger) checkAndRotate() {
	today := time.Now().Format("2006-01-02")
	if l.date != today {
		l.openTodayLog(config.GetLogsDir())
	}
}

func cleanupExpiredLogs(logsDir string, now time.Time) {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		fmt.Printf("读取日志目录失败: %v\n", err)
		return
	}

	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -7)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		logDate, ok := parseLogDate(entry.Name())
		if !ok || !logDate.Before(cutoff) {
			continue
		}

		logPath := filepath.Join(logsDir, entry.Name())
		if err := os.Remove(logPath); err != nil && !os.IsNotExist(err) {
			fmt.Printf("删除过期日志失败 %s: %v\n", logPath, err)
		}
	}
}

func parseLogDate(fileName string) (time.Time, bool) {
	if filepath.Ext(fileName) != ".log" {
		return time.Time{}, false
	}

	datePart := fileName[:len(fileName)-len(".log")]
	logDate, err := time.Parse("2006-01-02", datePart)
	if err != nil {
		return time.Time{}, false
	}

	return logDate, true
}

// log 内部日志记录方法
func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	l.checkAndRotate()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	levelName := levelNames[level]

	message := format
	if len(args) > 0 {
		message = fmt.Sprintf(format, args...)
	}

	logLine := fmt.Sprintf("[%s] [%s] %s\n", timestamp, levelName, message)

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.writer != nil {
		l.writer.Write([]byte(logLine))
	} else {
		// 如果 writer 未初始化，直接输出到控制台
		_, _ = originalStdout.Write([]byte(logLine))
	}
}

func (l *Logger) captured(level LogLevel, source string, message string) {
	message = strings.TrimRight(message, "\r\n")
	message = ansiEscapePattern.ReplaceAllString(message, "")
	if strings.TrimSpace(message) == "" {
		return
	}
	l.log(level, "[%s] %s", source, message)
}

func (l *Logger) SourceWriter(source string, level LogLevel) io.Writer {
	source = strings.TrimSpace(source)
	if source == "" {
		source = "EXTERNAL"
	}
	return sourceWriter{logger: l, source: source, level: level}
}

func NewSourceWriter(source string, level LogLevel) io.Writer {
	return GetLogger().SourceWriter(source, level)
}

type sourceWriter struct {
	logger *Logger
	source string
	level  LogLevel
}

func (w sourceWriter) Write(p []byte) (int, error) {
	if w.logger == nil {
		w.logger = GetLogger()
	}
	w.logger.captured(w.level, w.source, string(p))
	return len(p), nil
}

// Debug 记录调试日志
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DEBUG, format, args...)
}

// Info 记录信息日志
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, format, args...)
}

// Warning 记录警告日志
func (l *Logger) Warning(format string, args ...interface{}) {
	l.log(WARNING, format, args...)
}

// Error 记录错误日志
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
}

func (l *Logger) Vulnerability(format string, args ...interface{}) {
	l.log(VULN, format, args...)
}

// Debugf 记录调试日志（格式化）
func Debug(format string, args ...interface{}) {
	GetLogger().Debug(format, args...)
}

// Infof 记录信息日志（格式化）
func Info(format string, args ...interface{}) {
	GetLogger().Info(format, args...)
}

// Warningf 记录警告日志（格式化）
func Warning(format string, args ...interface{}) {
	GetLogger().Warning(format, args...)
}

// Errorf 记录错误日志（格式化）
func Error(format string, args ...interface{}) {
	GetLogger().Error(format, args...)
}

func Vulnerability(format string, args ...interface{}) {
	GetLogger().Vulnerability(format, args...)
}

// Close 关闭日志文件
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
