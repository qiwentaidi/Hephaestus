package webscan

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/logger"

	nucleitemplates "github.com/projectdiscovery/nuclei/v3/pkg/templates"
	"github.com/projectdiscovery/nuclei/v3/pkg/templates/signer"
	"github.com/rs/xid"
	"gopkg.in/yaml.v2"
)

const codeTemplateSignerID = "hephaestus-autosign"

var (
	codeTemplateSignerOnce sync.Once
	codeTemplateSignerErr  error
)

type codeTemplateSignStats struct {
	Total          int
	Signed         int
	SkippedEngines int
	SkippedParse   int
	SkippedNonCode int
	Failed         int
}

type codeTemplateProbe struct {
	Code []struct {
		Engine []string `yaml:"engine"`
	} `yaml:"code"`
}

// RegisterCodeTemplateSignerForNuclei registers the application-managed signer
// as a trusted verifier. It does not sign templates by itself; signing is only
// triggered from controlled PoC update paths.
func RegisterCodeTemplateSignerForNuclei() error {
	codeTemplateSignerOnce.Do(func() {
		tmplSigner, err := ensureCodeTemplateSigner()
		if err != nil {
			codeTemplateSignerErr = err
			return
		}
		codeTemplateSignerErr = signer.AddSignerToDefault(tmplSigner)
		if codeTemplateSignerErr == nil {
			ensureCodeTemplateSignatureStats(tmplSigner.Identifier())
		}
	})
	return codeTemplateSignerErr
}

func ensureCodeTemplateSignatureStats(verifier string) {
	verifier = strings.TrimSpace(verifier)
	if verifier == "" {
		return
	}
	if nucleitemplates.SignatureStats[verifier] == nil {
		nucleitemplates.SignatureStats[verifier] = &atomic.Uint64{}
	}
}

// SignUpdatedCodeTemplatesForNuclei removes old signatures and signs code
// templates under the supplied controlled update paths.
func SignUpdatedCodeTemplatesForNuclei(paths ...string) error {
	if err := RegisterCodeTemplateSignerForNuclei(); err != nil {
		return err
	}
	tmplSigner, err := ensureCodeTemplateSigner()
	if err != nil {
		return err
	}

	stats := codeTemplateSignStats{}
	if len(paths) == 0 {
		paths = []string{GetPoCDir()}
	}
	for _, path := range paths {
		if err := signUpdatedCodeTemplatePath(tmplSigner, strings.TrimSpace(path), &stats); err != nil {
			return err
		}
	}

	logger.Info("[nuclei] code template signing completed: total=%d signed=%d skipped_non_code=%d skipped_engine=%d skipped_parse=%d failed=%d",
		stats.Total, stats.Signed, stats.SkippedNonCode, stats.SkippedEngines, stats.SkippedParse, stats.Failed)
	return nil
}

func signUpdatedCodeTemplatePath(tmplSigner *signer.TemplateSigner, path string, stats *codeTemplateSignStats) error {
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		logger.Info("[nuclei] PoC update path does not exist, skip code template signing: %s", path)
		return nil
	}
	if err != nil {
		return err
	}

	nucleitemplates.TemplateSignerLFA()
	if !info.IsDir() {
		if !strings.EqualFold(filepath.Ext(path), ".yaml") {
			return nil
		}
		signCodeTemplateFileWithStats(tmplSigner, path, stats)
		return nil
	}

	return filepath.WalkDir(path, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(d.Name()), ".yaml") {
			return nil
		}
		signCodeTemplateFileWithStats(tmplSigner, path, stats)
		return nil
	})
}

func signCodeTemplateFileWithStats(tmplSigner *signer.TemplateSigner, path string, stats *codeTemplateSignStats) {
	stats.Total++
	signed, skipped, err := signCodeTemplateFile(tmplSigner, path)
	switch {
	case err != nil:
		stats.Failed++
		logger.Warning("[nuclei] code template signing failed for %s: %v", path, err)
	case skipped == "non-code":
		stats.SkippedNonCode++
	case skipped == "engine":
		stats.SkippedEngines++
		logger.Info("[nuclei] code template skipped due to unavailable engine: %s", path)
	case skipped == "parse":
		stats.SkippedParse++
		logger.Info("[nuclei] code template skipped due to parse/sign prerequisite: %s", path)
	case signed:
		stats.Signed++
	}
}

func signCodeTemplateFile(tmplSigner *signer.TemplateSigner, path string) (signed bool, skipped string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, "", err
	}

	probe, isCode, err := parseCodeTemplateProbe(data)
	if err != nil {
		return false, "parse", nil
	}
	if !isCode {
		return false, "non-code", nil
	}

	withoutDigest := removeTemplateDigestLines(data)
	if !bytes.Equal(withoutDigest, data) {
		if err := os.WriteFile(path, withoutDigest, 0o644); err != nil {
			return false, "", err
		}
	}

	if !codeTemplateEnginesAvailable(probe) {
		return false, "engine", nil
	}

	if err := nucleitemplates.SignTemplate(tmplSigner, path); err != nil {
		return false, "parse", nil
	}
	return true, "", nil
}

func parseCodeTemplateProbe(data []byte) (codeTemplateProbe, bool, error) {
	var probe codeTemplateProbe
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return probe, false, err
	}
	return probe, len(probe.Code) > 0, nil
}

func codeTemplateEnginesAvailable(probe codeTemplateProbe) bool {
	for _, request := range probe.Code {
		requestAvailable := false
		if len(request.Engine) == 0 {
			return false
		}
		for _, engine := range request.Engine {
			engine = strings.TrimSpace(engine)
			if engine == "" {
				continue
			}
			if _, err := exec.LookPath(engine); err == nil {
				requestAvailable = true
				break
			}
		}
		if !requestAvailable {
			return false
		}
	}
	return true
}

func removeTemplateDigestLines(data []byte) []byte {
	lines := strings.SplitAfter(string(data), "\n")
	var out strings.Builder
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), signer.SignaturePattern) {
			continue
		}
		out.WriteString(line)
	}
	return []byte(strings.TrimRight(out.String(), "\n") + "\n")
}

func ensureCodeTemplateSigner() (*signer.TemplateSigner, error) {
	keysDir := filepath.Join(config.GetConfigResourceRoot(), "nuclei-keys")
	certPath := filepath.Join(keysDir, signer.CertFilename)
	keyPath := filepath.Join(keysDir, signer.PrivateKeyFilename)

	certData, keyData, err := readSignerFiles(certPath, keyPath)
	if err == nil {
		if !isEncryptedPrivateKey(keyData) {
			if tmplSigner, err := signer.NewTemplateSigner(certData, keyData); err == nil {
				return tmplSigner, nil
			}
		}
		backupBrokenSignerFiles(certPath, keyPath)
	}

	certData, keyData, err = generateCodeTemplateSigner()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(keysDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(certPath, certData, 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, keyData, 0o600); err != nil {
		return nil, err
	}
	return signer.NewTemplateSigner(certData, keyData)
}

func isEncryptedPrivateKey(keyData []byte) bool {
	block, _ := pem.Decode(keyData)
	if block == nil {
		return true
	}
	return x509.IsEncryptedPEMBlock(block)
}

func readSignerFiles(certPath, keyPath string) ([]byte, []byte, error) {
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return nil, nil, err
	}
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, err
	}
	return certData, keyData, nil
}

func backupBrokenSignerFiles(certPath, keyPath string) {
	stamp := time.Now().Format("20060102150405")
	for _, path := range []string{certPath, keyPath} {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		_ = os.Rename(path, fmt.Sprintf("%s.invalid-%s", path, stamp))
	}
}

func generateCodeTemplateSigner() ([]byte, []byte, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(xid.New().Time().Unix()),
		Subject: pkix.Name{
			CommonName: codeTemplateSignerID,
		},
		SignatureAlgorithm:    x509.ECDSAWithSHA256,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(4 * 365 * 24 * time.Hour),
		PublicKey:             &privateKey.PublicKey,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: signer.CertType, Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: signer.PrivateKeyType, Bytes: keyDER})
	return certPEM, keyPEM, nil
}
