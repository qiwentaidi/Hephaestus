//go:build linux || darwin

// docker-updater runs on the Docker host and manages a single Compose project.
package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"hephaestus-backend/pkg/dockerupdate"
)

var (
	digestPattern  = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	projectPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
)

type job struct {
	dockerupdate.Status
	Digest string `json:"digest,omitempty"`
}

type updater struct {
	dir, project, stateDir string
	composeFiles           []string
	desktopAddress         string
	mu                     sync.Mutex
	job                    job
}

func main() {
	project := flag.String("project-name", "", "Existing Compose project name")
	dir := flag.String("project-directory", ".", "Directory containing the Compose files")
	activate := flag.Bool("activate", false, "Start the published image and connect it to this updater")
	desktopPort := flag.Int("desktop-port", 18182, "Loopback port used by Docker Desktop on macOS")
	flag.Parse()
	if err := run(*dir, *project, *activate, *desktopPort); err != nil {
		log.Fatal(err)
	}
}

func run(dir, project string, activate bool, desktopPort int) error {
	if !projectPattern.MatchString(project) {
		return errors.New("set -project-name to the deployment's Compose project name")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	u := &updater{dir: dir, project: project, stateDir: filepath.Join(dir, ".hephaestus-updater")}
	u.composeFiles = []string{"-f", "docker-compose.yml", "-f", "docker-compose.release.yml"}
	for _, override := range []string{"compose.override.yml", "compose.override.yaml", "docker-compose.override.yml", "docker-compose.override.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, override)); err == nil {
			u.composeFiles = append(u.composeFiles, "-f", override)
			break
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	ipc := filepath.Join(u.stateDir, "ipc")
	if err := os.MkdirAll(ipc, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(u.stateDir, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("lock updater state: %w", err)
	}

	data, err := os.ReadFile(filepath.Join(u.stateDir, "job.json"))
	if err == nil {
		if err := json.Unmarshal(data, &u.job); err != nil {
			return err
		}
		if u.job.Phase == dockerupdate.Pulling || u.job.Phase == dockerupdate.Restarting {
			u.job.Phase = dockerupdate.Failed
			u.job.Error = "Docker 更新器已重启，请重试更新"
			if err := u.saveJob(u.job); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	listener, token, err := u.listen(ipc, desktopPort)
	if err != nil {
		return err
	}
	defer listener.Close()

	if activate {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		err := u.activate(ctx)
		cancel()
		if err != nil {
			return err
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /release", u.check)
	mux.HandleFunc("GET /status", u.status)
	mux.HandleFunc("POST /pull", u.startPull)
	mux.HandleFunc("POST /restart", u.startRestart)
	var handler http.Handler = mux
	if token != "" {
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
				reply(w, http.StatusUnauthorized, dockerupdate.Status{Error: "unauthorized"})
				return
			}
			mux.ServeHTTP(w, r)
		})
	}
	server := &http.Server{Handler: handler, ReadTimeout: 5 * time.Second, WriteTimeout: 25 * time.Second}
	log.Printf("Docker updater serving Compose project %s", project)
	return server.Serve(listener)
}

func (u *updater) listen(ipc string, port int) (net.Listener, string, error) {
	if runtime.GOOS == "darwin" {
		// Docker Desktop shares files from macOS, but not live Unix sockets.
		if port < 1 || port > 65535 {
			return nil, "", errors.New("invalid Docker Desktop updater port")
		}
		tokenPath := filepath.Join(ipc, "token")
		token, err := os.ReadFile(tokenPath)
		if os.IsNotExist(err) {
			key := make([]byte, 32)
			if _, err := rand.Read(key); err != nil {
				return nil, "", err
			}
			token = []byte(hex.EncodeToString(key))
			err = writeAtomic(tokenPath, token)
		}
		if err != nil {
			return nil, "", err
		}
		if _, err := hex.DecodeString(string(token)); err != nil || len(token) != 64 {
			return nil, "", errors.New("invalid Docker Desktop updater token")
		}
		u.desktopAddress = "host.docker.internal:" + strconv.Itoa(port)
		listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
		return listener, string(token), err
	}

	socket := filepath.Join(ipc, "control.sock")
	if err := os.Remove(socket); err != nil && !os.IsNotExist(err) {
		return nil, "", err
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return nil, "", err
	}
	if err := os.Chmod(socket, 0600); err != nil {
		listener.Close()
		return nil, "", err
	}
	return listener, "", nil
}

func (u *updater) docker(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = u.dir
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("docker %s: %w", args[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (u *updater) latest(ctx context.Context) (dockerupdate.Release, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	raw, err := u.docker(ctx, "buildx", "imagetools", "inspect", "--format", "{{json .Manifest}}", dockerupdate.ImageRepository+":latest")
	if err != nil {
		return dockerupdate.Release{}, err
	}
	var manifest struct {
		Digest      string            `json:"digest"`
		Annotations map[string]string `json:"annotations"`
	}
	if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
		return dockerupdate.Release{}, err
	}
	release := dockerupdate.Release{Version: manifest.Annotations[dockerupdate.VersionLabel], Digest: manifest.Digest}
	if created, err := time.Parse(time.RFC3339, manifest.Annotations["org.opencontainers.image.created"]); err == nil {
		release.ReleaseDate = created.Format(time.DateOnly)
	}
	if !dockerupdate.ValidVersion(release.Version) || !digestPattern.MatchString(release.Digest) {
		return dockerupdate.Release{}, errors.New("published image has no valid release version or digest")
	}
	return release, nil
}

func (u *updater) imageVersion(ctx context.Context, image string) (string, error) {
	return u.docker(ctx, "image", "inspect", "--format", `{{index .Config.Labels "org.opencontainers.image.version"}}`, image)
}

func (u *updater) pull(ctx context.Context, release dockerupdate.Release) error {
	image := dockerupdate.ImageRepository + "@" + release.Digest
	if _, err := u.docker(ctx, "pull", image); err != nil {
		return err
	}
	return u.verifyImage(ctx, image, release.Version)
}

func (u *updater) verifyImage(ctx context.Context, image, expectedVersion string) error {
	version, err := u.imageVersion(ctx, image)
	if err != nil {
		return err
	}
	if version != expectedVersion {
		return fmt.Errorf("image version %q does not match %q", version, expectedVersion)
	}
	return nil
}

func (u *updater) compose(ctx context.Context, image string, args ...string) (string, error) {
	composeArgs := append([]string{"compose", "-p", u.project}, u.composeFiles...)
	cmd := exec.CommandContext(ctx, "docker", append(composeArgs, args...)...)
	cmd.Dir = u.dir
	cmd.Env = append(os.Environ(), "HEPHAESTUS_IMAGE="+image, dockerupdate.AddressEnv+"="+u.desktopAddress)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("docker compose %s: %w", args[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (u *updater) apply(ctx context.Context, release dockerupdate.Release) error {
	image := dockerupdate.ImageRepository + "@" + release.Digest
	if err := u.verifyImage(ctx, image, release.Version); err != nil {
		return err
	}

	// Store the desired image before Compose starts replacing the container.
	if err := writeAtomic(filepath.Join(u.stateDir, "image.env"), []byte("HEPHAESTUS_IMAGE="+image+"\n"+dockerupdate.AddressEnv+"="+u.desktopAddress+"\n")); err != nil {
		return err
	}
	if _, err := u.compose(ctx, image, "up", "-d", "--no-build", "--pull", "never", "--wait", "--wait-timeout", "240", "hephaestus"); err != nil {
		return err
	}
	container, err := u.compose(ctx, image, "ps", "-q", "hephaestus")
	if err != nil || container == "" {
		return errors.New("could not identify the upgraded service")
	}
	running, err := u.docker(ctx, "inspect", "--format", "{{.Image}}", container)
	if err != nil {
		return err
	}
	expected, err := u.docker(ctx, "image", "inspect", "--format", "{{.Id}}", image)
	if err != nil {
		return err
	}
	if running != expected {
		return errors.New("the running container does not use the staged image")
	}
	return nil
}

func (u *updater) activate(ctx context.Context) error {
	// Reuse the persisted target when restarting an already managed deployment.
	data, err := os.ReadFile(filepath.Join(u.stateDir, "image.env"))
	if err == nil {
		line, _, _ := strings.Cut(string(data), "\n")
		digest := strings.TrimPrefix(line, "HEPHAESTUS_IMAGE="+dockerupdate.ImageRepository+"@")
		if !digestPattern.MatchString(digest) {
			return errors.New("invalid saved image reference")
		}
		image := dockerupdate.ImageRepository + "@" + digest
		if _, err := u.docker(ctx, "pull", image); err != nil {
			return err
		}
		version, err := u.imageVersion(ctx, image)
		if err != nil || !dockerupdate.ValidVersion(version) {
			return errors.New("saved image has no valid release version")
		}
		return u.apply(ctx, dockerupdate.Release{Version: version, Digest: digest})
	}
	if !os.IsNotExist(err) {
		return err
	}

	release, err := u.latest(ctx)
	if err != nil {
		return err
	}
	if err := u.pull(ctx, release); err != nil {
		return err
	}
	return u.apply(ctx, release)
}

func (u *updater) check(w http.ResponseWriter, r *http.Request) {
	release, err := u.latest(r.Context())
	if err != nil {
		log.Printf("check Docker release: %v", err)
		reply(w, http.StatusBadGateway, dockerupdate.Status{Error: "无法获取已发布的 Docker 镜像版本"})
		return
	}
	reply(w, http.StatusOK, release)
}

func (u *updater) status(w http.ResponseWriter, _ *http.Request) {
	u.mu.Lock()
	defer u.mu.Unlock()
	reply(w, http.StatusOK, u.job.Status)
}

func readVersion(w http.ResponseWriter, r *http.Request) string {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var request dockerupdate.Release
	dec := json.NewDecoder(r.Body)
	if dec.Decode(&request) != nil || dec.Decode(new(any)) != io.EOF || !dockerupdate.ValidVersion(request.Version) {
		reply(w, http.StatusBadRequest, dockerupdate.Status{Error: "无效的 Docker 更新版本"})
		return ""
	}
	return request.Version
}

func (u *updater) startPull(w http.ResponseWriter, r *http.Request) {
	version := readVersion(w, r)
	if version == "" {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.job.Version == version && (u.job.Phase == dockerupdate.Pulling || u.job.Phase == dockerupdate.Staged) {
		reply(w, http.StatusOK, u.job.Status)
		return
	}
	next := job{Status: dockerupdate.Status{Version: version, Phase: dockerupdate.Pulling}}
	if !u.begin(w, next) {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		release, err := u.latest(ctx)
		if err == nil && release.Version != version {
			err = errors.New("requested version is no longer the current release")
		}
		if err == nil {
			err = u.pull(ctx, release)
		}
		if err != nil {
			u.finish(next, dockerupdate.Failed, "Docker 镜像下载失败，请重新检查更新", err)
			return
		}
		next.Digest = release.Digest
		u.finish(next, dockerupdate.Staged, "", nil)
	}()
}

func (u *updater) startRestart(w http.ResponseWriter, r *http.Request) {
	version := readVersion(w, r)
	if version == "" {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.job.Version == version && u.job.Phase == dockerupdate.Restarting {
		reply(w, http.StatusOK, u.job.Status)
		return
	}
	if u.job.Version != version || !digestPattern.MatchString(u.job.Digest) ||
		(u.job.Phase != dockerupdate.Staged && u.job.Phase != dockerupdate.Failed) {
		reply(w, http.StatusConflict, dockerupdate.Status{Error: "请先下载对应版本的 Docker 镜像"})
		return
	}
	next := u.job
	next.Phase, next.Error = dockerupdate.Restarting, ""
	if !u.begin(w, next) {
		return
	}

	go func() {
		// Allow the Web server to return the accepted response before it stops.
		time.Sleep(1200 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
		defer cancel()
		if err := u.apply(ctx, dockerupdate.Release{Version: version, Digest: next.Digest}); err != nil {
			u.finish(next, dockerupdate.Failed, "Docker 容器更新失败，请查看宿主机更新器日志", err)
			return
		}
		u.finish(next, dockerupdate.Ready, "", nil)
	}()
}

func (u *updater) begin(w http.ResponseWriter, next job) bool {
	if u.job.Phase == dockerupdate.Pulling || u.job.Phase == dockerupdate.Restarting {
		reply(w, http.StatusConflict, dockerupdate.Status{Error: "Docker 更新正在进行中"})
		return false
	}
	if err := u.saveJob(next); err != nil {
		log.Printf("save Docker update state: %v", err)
		reply(w, http.StatusInternalServerError, dockerupdate.Status{Error: "无法保存 Docker 更新状态"})
		return false
	}
	reply(w, http.StatusOK, u.job.Status)
	return true
}

func (u *updater) finish(next job, phase, message string, cause error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if cause != nil {
		log.Printf("Docker update %s: %v", next.Version, cause)
	}
	next.Phase, next.Error = phase, message
	if err := u.saveJob(next); err != nil {
		log.Printf("save Docker update state: %v", err)
		next.Phase, next.Error = dockerupdate.Failed, "无法保存 Docker 更新状态"
		u.job = next
	}
}

func (u *updater) saveJob(next job) error {
	data, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(u.stateDir, "job.json"), data); err != nil {
		return err
	}
	u.job = next
	return nil
}

func writeAtomic(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".update-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	_, writeErr := temp.Write(data)
	if err := errors.Join(writeErr, temp.Close()); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}

func reply(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}
