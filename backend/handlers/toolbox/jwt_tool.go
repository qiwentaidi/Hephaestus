package toolbox

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"hephaestus-backend/pkg/config"
)

type JWTBruteforceRequest struct {
	Token             string `json:"token" binding:"required"`
	Dictionary        string `json:"dictionary" binding:"required"`
	Source            string `json:"source"`
	TryBase64Variants bool   `json:"tryBase64Variants"`
}

type parsedJWTForBruteforce struct {
	SigningInput   string
	SignatureBytes []byte
	Algorithm      string
}

type jwtKeyVariant struct {
	Label                 string
	DisplayValue          string
	Bytes                 []byte
	Base64URLDecoded      bool
	UsedFixedLengthSecret bool
}

type JWTBruteforceResponse struct {
	Matched                  bool   `json:"matched"`
	Attempts                 int    `json:"attempts"`
	ElapsedMs                int64  `json:"elapsedMs"`
	TotalCandidates          int    `json:"totalCandidates"`
	DictionaryEntry          string `json:"dictionaryEntry,omitempty"`
	VariantLabel             string `json:"variantLabel,omitempty"`
	DisplayValue             string `json:"displayValue,omitempty"`
	SecretPreview            string `json:"secretPreview,omitempty"`
	SecretIsBase64URLEncoded bool   `json:"secretIsBase64UrlEncoded,omitempty"`
	UsedFixedLengthSecret    bool   `json:"usedFixedLengthSecret,omitempty"`
	UnsupportedAlgorithm     string `json:"unsupportedAlgorithm,omitempty"`
	Aborted                  bool   `json:"aborted,omitempty"`
}

// JWTDictionaryInfo is the safe, client-facing description of a managed JWT
// dictionary. Paths are intentionally omitted: callers select by basename.
type JWTDictionaryInfo struct {
	Name   string `json:"name"`
	Custom bool   `json:"custom"`
}

// ListJWTDictionariesTool returns the .txt dictionaries available to JWT
// testing. The result is deliberately limited to managed basenames so an AI
// caller cannot depend on a server-local absolute path.
func ListJWTDictionariesTool() ([]JWTDictionaryInfo, error) {
	files, err := config.ListManagedDictFiles("jwt")
	if err != nil {
		return nil, err
	}
	result := make([]JWTDictionaryInfo, 0, len(files))
	for _, file := range files {
		result = append(result, JWTDictionaryInfo{Name: file.Name, Custom: file.Custom})
	}
	return result, nil
}

func RunJWTBruteforce(ctx context.Context, req JWTBruteforceRequest) (JWTBruteforceResponse, error) {
	parsed, err := parseJWTForBruteforce(req.Token)
	if err != nil {
		return JWTBruteforceResponse{}, err
	}

	if !isSupportedJWTHMACAlgorithm(parsed.Algorithm) {
		return JWTBruteforceResponse{
			Matched:              false,
			Attempts:             0,
			ElapsedMs:            0,
			TotalCandidates:      0,
			UnsupportedAlgorithm: parsed.Algorithm,
		}, nil
	}

	sourceKey := strings.TrimSpace(req.Source)
	if sourceKey == "" {
		sourceKey = "jwt"
	}
	source, err := getManagedDictSource(sourceKey)
	if err != nil {
		return JWTBruteforceResponse{}, err
	}
	// Keep the internal AI path resilient if it omits the name after the
	// discovery call: choose the first deterministic managed .txt dictionary.
	if strings.TrimSpace(req.Dictionary) == "" {
		files, listErr := config.ListManagedDictFiles(source.Key)
		if listErr != nil {
			return JWTBruteforceResponse{}, errors.New("加载 JWT 字典列表失败: " + listErr.Error())
		}
		if len(files) == 0 {
			return JWTBruteforceResponse{}, errors.New("没有可用的 JWT .txt 字典，请先在字典管理中上传字典")
		}
		req.Dictionary = files[0].Name
	}

	entries, err := loadJWTDictionaryEntries(source, strings.TrimSpace(req.Dictionary))
	if err != nil {
		return JWTBruteforceResponse{}, errors.New("加载 JWT 字典失败: " + err.Error())
	}
	if len(entries) == 0 {
		return JWTBruteforceResponse{}, errors.New("所选字典为空，无法进行爆破")
	}

	uniqueEntries := make([]string, 0, len(entries))
	seenEntries := make(map[string]struct{}, len(entries))
	totalCandidates := 0
	for _, entry := range entries {
		if _, exists := seenEntries[entry]; exists {
			continue
		}
		seenEntries[entry] = struct{}{}
		uniqueEntries = append(uniqueEntries, entry)
		variants := buildJWTKeyVariants(entry, req.TryBase64Variants)
		for _, variant := range variants {
			totalCandidates += len(getJWTHMACSecretCandidates(parsed.Algorithm, variant.Bytes))
		}
	}

	startedAt := time.Now()
	attempts := 0
	if ctx == nil {
		ctx = context.Background()
	}

	for _, entry := range uniqueEntries {
		if isJWTBruteforceCancelled(ctx) {
			return JWTBruteforceResponse{
				Matched:         false,
				Attempts:        attempts,
				ElapsedMs:       time.Since(startedAt).Milliseconds(),
				TotalCandidates: totalCandidates,
				Aborted:         true,
			}, nil
		}

		variants := buildJWTKeyVariants(entry, req.TryBase64Variants)
		for _, variant := range variants {
			if isJWTBruteforceCancelled(ctx) {
				return JWTBruteforceResponse{
					Matched:         false,
					Attempts:        attempts,
					ElapsedMs:       time.Since(startedAt).Milliseconds(),
					TotalCandidates: totalCandidates,
					Aborted:         true,
				}, nil
			}

			for _, secretCandidate := range getJWTHMACSecretCandidates(parsed.Algorithm, variant.Bytes) {
				if isJWTBruteforceCancelled(ctx) {
					return JWTBruteforceResponse{
						Matched:         false,
						Attempts:        attempts,
						ElapsedMs:       time.Since(startedAt).Milliseconds(),
						TotalCandidates: totalCandidates,
						Aborted:         true,
					}, nil
				}

				attempts++
				signature, err := signJWTHMAC(
					parsed.Algorithm,
					parsed.SigningInput,
					secretCandidate.Bytes,
				)
				if err != nil {
					return JWTBruteforceResponse{}, errors.New("JWT 签名计算失败: " + err.Error())
				}

				if hmac.Equal(signature, parsed.SignatureBytes) {
					variantLabel := variant.Label
					if secretCandidate.Label != "标准 HMAC" {
						variantLabel = variant.Label + " + " + secretCandidate.Label
					}
					return JWTBruteforceResponse{
						Matched:                  true,
						Attempts:                 attempts,
						ElapsedMs:                time.Since(startedAt).Milliseconds(),
						TotalCandidates:          totalCandidates,
						DictionaryEntry:          entry,
						VariantLabel:             variantLabel,
						DisplayValue:             variant.DisplayValue,
						SecretPreview:            encodeBase64URLBytes(secretCandidate.Bytes),
						SecretIsBase64URLEncoded: variant.Base64URLDecoded,
						UsedFixedLengthSecret:    secretCandidate.UsedFixedLengthSecret,
					}, nil
				}
			}
		}
	}

	return JWTBruteforceResponse{
		Matched:         false,
		Attempts:        attempts,
		ElapsedMs:       time.Since(startedAt).Milliseconds(),
		TotalCandidates: totalCandidates,
	}, nil
}

func RunJWTBruteforceTool(ctx context.Context, token, dictionary, source string, tryBase64Variants bool) (any, error) {
	return RunJWTBruteforce(ctx, JWTBruteforceRequest{
		Token:             token,
		Dictionary:        dictionary,
		Source:            source,
		TryBase64Variants: tryBase64Variants,
	})
}

var jwtFixedSecretLengthMap = map[string]int{
	"HS256": 32,
	"HS384": 48,
	"HS512": 64,
}

var jwtBase64CandidatePattern = regexp.MustCompile(`^[A-Za-z0-9+/=_-]+$`)

func parseJWTForBruteforce(raw string) (*parsedJWTForBruteforce, error) {
	trimmed := strings.TrimSpace(raw)
	parts := strings.Split(trimmed, ".")
	if len(parts) != 3 {
		return nil, errors.New("JWT 必须由 3 段组成")
	}

	headerBytes, err := decodeBase64URLBytes(parts[0])
	if err != nil {
		return nil, errors.New("JWT Header 解码失败")
	}

	var header map[string]any
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, errors.New("JWT Header 不是有效的 JSON 对象")
	}

	algorithm, _ := header["alg"].(string)
	if strings.TrimSpace(algorithm) == "" {
		return nil, errors.New("JWT Header 缺少 alg 字段")
	}

	signatureBytes, err := decodeBase64URLBytes(parts[2])
	if err != nil {
		return nil, errors.New("JWT Signature 解码失败")
	}

	return &parsedJWTForBruteforce{
		SigningInput:   parts[0] + "." + parts[1],
		SignatureBytes: signatureBytes,
		Algorithm:      strings.ToUpper(strings.TrimSpace(algorithm)),
	}, nil
}

func decodeBase64URLBytes(input string) ([]byte, error) {
	normalized := strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(input), "-", "+"), "_", "/")
	if mod := len(normalized) % 4; mod != 0 {
		normalized += strings.Repeat("=", 4-mod)
	}
	return base64.StdEncoding.DecodeString(normalized)
}

func encodeBase64URLBytes(input []byte) string {
	return base64.RawURLEncoding.EncodeToString(input)
}

func bytesToHexString(input []byte) string {
	return hex.EncodeToString(input)
}

func tryDecodeBase64Candidate(input string) ([]byte, bool) {
	normalized := strings.ReplaceAll(strings.TrimSpace(input), " ", "")
	normalized = strings.ReplaceAll(normalized, "\n", "")
	normalized = strings.ReplaceAll(normalized, "\r", "")
	normalized = strings.ReplaceAll(normalized, "\t", "")
	if normalized == "" || !jwtBase64CandidatePattern.MatchString(normalized) {
		return nil, false
	}

	decoded, err := decodeBase64URLBytes(normalized)
	if err != nil {
		return nil, false
	}
	return decoded, true
}

func tryDecodeHexCandidate(input string) ([]byte, bool) {
	normalized := strings.TrimSpace(input)
	normalized = strings.TrimPrefix(strings.TrimPrefix(normalized, "0x"), "0X")
	if normalized == "" || len(normalized)%2 != 0 {
		return nil, false
	}
	decoded, err := hex.DecodeString(normalized)
	if err != nil {
		return nil, false
	}
	return decoded, true
}

func unwrapQuotedJWTSecret(input string) string {
	trimmed := strings.TrimSpace(input)
	if len(trimmed) < 2 {
		return trimmed
	}
	first, last := trimmed[0], trimmed[len(trimmed)-1]
	if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
		return strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	}
	return trimmed
}

func addJWTVariant(variants *[]jwtKeyVariant, seen map[string]struct{}, label string, displayValue string, bytes []byte, base64URLDecoded bool) {
	if len(bytes) == 0 {
		return
	}
	key := bytesToHexString(bytes)
	if _, exists := seen[key]; exists {
		return
	}
	seen[key] = struct{}{}
	*variants = append(*variants, jwtKeyVariant{
		Label:            label,
		DisplayValue:     displayValue,
		Bytes:            bytes,
		Base64URLDecoded: base64URLDecoded,
	})
}

func buildJWTKeyVariants(entry string, includeBase64Variants bool) []jwtKeyVariant {
	normalized := unwrapQuotedJWTSecret(entry)
	if normalized == "" {
		return nil
	}

	var variants []jwtKeyVariant
	seen := make(map[string]struct{})

	addJWTVariant(&variants, seen, "原始文本", normalized, []byte(normalized), false)
	if normalized != strings.TrimSpace(entry) {
		addJWTVariant(&variants, seen, "去除包裹引号", normalized, []byte(normalized), false)
	}

	if includeBase64Variants {
		if decoded, ok := tryDecodeBase64Candidate(normalized); ok {
			addJWTVariant(&variants, seen, "Base64/Base64URL 还原", normalized, decoded, true)
		}
	}

	if decodedHex, ok := tryDecodeHexCandidate(normalized); ok {
		addJWTVariant(&variants, seen, "Hex 还原", normalized, decodedHex, false)
	}

	if idx := strings.Index(normalized, ":"); idx >= 0 {
		passwordPart := strings.TrimSpace(normalized[idx+1:])
		if passwordPart != "" {
			addJWTVariant(&variants, seen, "冒号后半段", passwordPart, []byte(passwordPart), false)
			if includeBase64Variants {
				if decoded, ok := tryDecodeBase64Candidate(passwordPart); ok {
					addJWTVariant(&variants, seen, "后半段 Base64 还原", passwordPart, decoded, true)
				}
			}
			if decodedHex, ok := tryDecodeHexCandidate(passwordPart); ok {
				addJWTVariant(&variants, seen, "后半段 Hex 还原", passwordPart, decodedHex, false)
			}
		}
	}

	return variants
}

func signJWTHMAC(algorithm string, signingInput string, secret []byte) ([]byte, error) {
	var mac hashProvider
	switch strings.ToUpper(strings.TrimSpace(algorithm)) {
	case "HS256":
		mac = sha256.New
	case "HS384":
		mac = sha512.New384
	case "HS512":
		mac = sha512.New
	default:
		return nil, errors.New("暂不支持的 JWT 算法")
	}

	hasher := hmac.New(mac, secret)
	_, _ = hasher.Write([]byte(signingInput))
	return hasher.Sum(nil), nil
}

func normalizeJWTHMACSecret(algorithm string, secret []byte) []byte {
	expectedLength, ok := jwtFixedSecretLengthMap[strings.ToUpper(strings.TrimSpace(algorithm))]
	if !ok || expectedLength <= 0 || len(secret) == expectedLength {
		return secret
	}

	normalized := make([]byte, expectedLength)
	copy(normalized, secret[:min(len(secret), expectedLength)])
	return normalized
}

func getJWTHMACSecretCandidates(algorithm string, secret []byte) []jwtKeyVariant {
	candidates := make([]jwtKeyVariant, 0, 2)
	seen := make(map[string]struct{}, 2)

	addCandidate := func(label string, bytes []byte, usedFixedLengthSecret bool) {
		key := bytesToHexString(bytes)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		candidates = append(candidates, jwtKeyVariant{
			Label:                 label,
			Bytes:                 bytes,
			Base64URLDecoded:      false,
			UsedFixedLengthSecret: usedFixedLengthSecret,
		})
	}

	addCandidate("标准 HMAC", secret, false)
	addCandidate("固定长度兼容", normalizeJWTHMACSecret(algorithm, secret), true)
	return candidates
}

type hashProvider func() hash.Hash

func extractJWTDictionaryEntries(content string) []string {
	lines := strings.Split(content, "\n")
	entries := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		entries = append(entries, trimmed)
	}
	return entries
}

func loadJWTDictionaryEntries(source managedDictSource, name string) ([]string, error) {
	if err := source.Ensure(); err != nil {
		return nil, err
	}
	if err := ValidateDictFileName(name); err != nil {
		return nil, err
	}

	file, err := config.ResolveManagedDictFile(source.Key, name)
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(file.Path)
	if err != nil {
		return nil, err
	}

	return extractJWTDictionaryEntries(string(content)), nil
}

func isJWTBruteforceCancelled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// JWTBruteforceHandler 在后端执行 JWT HMAC Secret 字典爆破。
func JWTBruteforceHandler(c *gin.Context) {
	var req JWTBruteforceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}

	parsed, err := parseJWTForBruteforce(req.Token)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if !isSupportedJWTHMACAlgorithm(parsed.Algorithm) {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": JWTBruteforceResponse{
				Matched:              false,
				Attempts:             0,
				ElapsedMs:            0,
				TotalCandidates:      0,
				UnsupportedAlgorithm: parsed.Algorithm,
			},
		})
		return
	}

	sourceKey := strings.TrimSpace(req.Source)
	if sourceKey == "" {
		sourceKey = "jwt"
	}
	source, err := getManagedDictSource(sourceKey)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	entries, err := loadJWTDictionaryEntries(source, strings.TrimSpace(req.Dictionary))
	if err != nil {
		status := http.StatusInternalServerError
		if os.IsNotExist(err) || strings.Contains(err.Error(), "字典") || strings.Contains(err.Error(), "非法") {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{
			"success": false,
			"error":   "加载 JWT 字典失败: " + err.Error(),
		})
		return
	}
	if len(entries) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "所选字典为空，无法进行爆破",
		})
		return
	}

	uniqueEntries := make([]string, 0, len(entries))
	seenEntries := make(map[string]struct{}, len(entries))
	totalCandidates := 0
	for _, entry := range entries {
		if _, exists := seenEntries[entry]; exists {
			continue
		}
		seenEntries[entry] = struct{}{}
		uniqueEntries = append(uniqueEntries, entry)
		variants := buildJWTKeyVariants(entry, req.TryBase64Variants)
		for _, variant := range variants {
			totalCandidates += len(getJWTHMACSecretCandidates(parsed.Algorithm, variant.Bytes))
		}
	}

	startedAt := time.Now()
	attempts := 0
	ctx := c.Request.Context()

	for _, entry := range uniqueEntries {
		if isJWTBruteforceCancelled(ctx) {
			return
		}

		variants := buildJWTKeyVariants(entry, req.TryBase64Variants)
		for _, variant := range variants {
			if isJWTBruteforceCancelled(ctx) {
				return
			}

			for _, secretCandidate := range getJWTHMACSecretCandidates(parsed.Algorithm, variant.Bytes) {
				if isJWTBruteforceCancelled(ctx) {
					return
				}

				attempts++
				signature, err := signJWTHMAC(
					parsed.Algorithm,
					parsed.SigningInput,
					secretCandidate.Bytes,
				)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{
						"success": false,
						"error":   "JWT 签名计算失败: " + err.Error(),
					})
					return
				}

				if hmac.Equal(signature, parsed.SignatureBytes) {
					variantLabel := variant.Label
					if secretCandidate.Label != "标准 HMAC" {
						variantLabel = variant.Label + " + " + secretCandidate.Label
					}
					c.JSON(http.StatusOK, gin.H{
						"success": true,
						"data": JWTBruteforceResponse{
							Matched:                  true,
							Attempts:                 attempts,
							ElapsedMs:                time.Since(startedAt).Milliseconds(),
							TotalCandidates:          totalCandidates,
							DictionaryEntry:          entry,
							VariantLabel:             variantLabel,
							DisplayValue:             variant.DisplayValue,
							SecretPreview:            encodeBase64URLBytes(secretCandidate.Bytes),
							SecretIsBase64URLEncoded: variant.Base64URLDecoded,
							UsedFixedLengthSecret:    secretCandidate.UsedFixedLengthSecret,
						},
					})
					return
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": JWTBruteforceResponse{
			Matched:         false,
			Attempts:        attempts,
			ElapsedMs:       time.Since(startedAt).Milliseconds(),
			TotalCandidates: totalCandidates,
		},
	})
}
func isSupportedJWTHMACAlgorithm(algorithm string) bool {
	switch strings.ToUpper(strings.TrimSpace(algorithm)) {
	case "HS256", "HS384", "HS512":
		return true
	default:
		return false
	}
}
