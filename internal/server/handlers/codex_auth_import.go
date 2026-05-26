package handlers

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/provider/auth"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/gin-gonic/gin"
)

const (
	maxSingleFileSize   = 1 << 20 // 1 MB
	maxZipEntries       = 50
	maxZipTotalJSONSize = 10 << 20 // 10 MB
	maxTotalFiles       = 200      // 单次导入最大文件数
)

func init() {
	router.NewGroupRouter("/api/v1/channel").
		Use(middleware.Auth()).
		AddRoute(
			router.NewRoute("/:id/codex/auth-files/import", http.MethodPost).
				Handle(importCodexAuthFiles),
		)
}

// codexAuthFile CPA Codex auth JSON 文件格式
type codexAuthFile struct {
	Type               string            `json:"type"`
	IDToken            string            `json:"id_token"`
	AccessToken        string            `json:"access_token"`
	RefreshToken       string            `json:"refresh_token"`
	AccountID          string            `json:"account_id"`
	Email              string            `json:"email"`
	Expired            json.RawMessage   `json:"expired"`
	Disabled           bool              `json:"disabled"`
	Priority           int               `json:"priority"`
	Note               string            `json:"note"`
	ProxyURL           string            `json:"proxy_url"`
	Prefix             string            `json:"prefix"`
	Headers            map[string]string `json:"headers"`
	RequestRetry       int               `json:"request_retry"`
	DisableCooling     bool              `json:"disable_cooling"`
	ToolPrefixDisabled bool              `json:"tool_prefix_disabled"`
	// 兼容 4 种 last_refresh key 名
	LastRefresh string `json:"last_refresh"`
}

// importResult 单个文件的导入结果
type importResult struct {
	File      string `json:"file"`
	Source    string `json:"source"`
	Status    string `json:"status"`
	Email     string `json:"email,omitempty"`
	AccountID string `json:"account_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

// importBatchResult 批量导入结果
type importBatchResult struct {
	Imported int            `json:"imported"`
	Updated  int            `json:"updated"`
	Failed   int            `json:"failed"`
	Skipped  int            `json:"skipped"`
	Results  []importResult `json:"results"`
}

func importCodexAuthFiles(c *gin.Context) {
	// 解析 channel ID
	idStr := c.Param("id")
	channelID, err := strconv.Atoi(idStr)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, "invalid channel id")
		return
	}

	// 验证渠道存在且是 codex 类型
	channel, err := op.ChannelGet(channelID, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusNotFound, "channel not found")
		return
	}
	if channel.ProviderID != "codex" {
		resp.Error(c, http.StatusBadRequest, "channel is not a codex provider")
		return
	}

	// 限制请求体大小（总上传 32MB）
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<20)

	form, err := c.MultipartForm()
	if err != nil {
		resp.Error(c, http.StatusBadRequest, "failed to parse multipart form")
		return
	}

	// 收集所有文件：兼容 "files" 和 "file" 字段名
	files := form.File["files"]
	if singleFiles := form.File["file"]; len(singleFiles) > 0 {
		files = append(files, singleFiles...)
	}
	if len(files) == 0 {
		resp.Error(c, http.StatusBadRequest, "no files uploaded")
		return
	}
	if len(files) > maxTotalFiles {
		resp.Error(c, http.StatusBadRequest, fmt.Sprintf("too many files (%d), max %d per import", len(files), maxTotalFiles))
		return
	}

	result := importBatchResult{
		Results: make([]importResult, 0),
	}

	// 预构建 key 查找映射，避免 N+1 credential 解析
	keyLookup := buildKeyLookup(channel)

	// 用于 zip 内 basename 去重
	zipBasenames := make(map[string]bool)

	for _, fh := range files {
		lowerName := strings.ToLower(fh.Filename)
		if strings.HasSuffix(lowerName, ".zip") {
			// 处理 ZIP 文件
			if err := processZipFile(fh, channelID, channel, &result, zipBasenames, keyLookup); err != nil {
				result.Results = append(result.Results, importResult{
					File:   fh.Filename,
					Source: "zip",
					Status: "failed",
					Error:  err.Error(),
				})
				result.Failed++
			}
		} else if strings.HasSuffix(lowerName, ".json") {
			// 处理单个 JSON 文件
			if fh.Size > maxSingleFileSize {
				result.Results = append(result.Results, importResult{
					File:   fh.Filename,
					Source: "json",
					Status: "failed",
					Error:  "file exceeds 1MB size limit",
				})
				result.Failed++
				continue
			}
			f, err := fh.Open()
			if err != nil {
				result.Results = append(result.Results, importResult{
					File:   fh.Filename,
					Source: "json",
					Status: "failed",
					Error:  "failed to open file",
				})
				result.Failed++
				continue
			}
			data, err := io.ReadAll(io.LimitReader(f, maxSingleFileSize+1))
			f.Close()
			if err != nil {
				result.Results = append(result.Results, importResult{
					File:   fh.Filename,
					Source: "json",
					Status: "failed",
					Error:  "failed to read file",
				})
				result.Failed++
				continue
			}
			if len(data) > maxSingleFileSize {
				result.Results = append(result.Results, importResult{
					File:   fh.Filename,
					Source: "json",
					Status: "failed",
					Error:  "file exceeds 1MB size limit",
				})
				result.Failed++
				continue
			}
			basename := path.Base(fh.Filename)
			processJSONData(data, basename, "json", channelID, channel, &result, nil, keyLookup)
		} else {
			result.Results = append(result.Results, importResult{
				File:   fh.Filename,
				Source: "json",
				Status: "skipped",
				Error:  "not a json or zip file",
			})
			result.Skipped++
		}
	}

	// 导入成功后自动创建 usage card（异步，不阻塞响应）
	if result.Imported > 0 || result.Updated > 0 {
		go op.AutoCreateCodexUsageCard(channel, context.Background())
	}

	resp.Success(c, result)
}

func processZipFile(fh *multipart.FileHeader, channelID int, channel *model.Channel, result *importBatchResult, zipBasenames map[string]bool, keyLookup map[string]int) error {
	if fh.Size > maxSingleFileSize*10 { // zip 本身限制 10MB
		return fmt.Errorf("zip file exceeds 10MB size limit")
	}

	f, err := fh.Open()
	if err != nil {
		return fmt.Errorf("failed to open zip file")
	}
	defer f.Close()

	// 将 zip 内容读入内存（已限制大小）
	zipData, err := io.ReadAll(io.LimitReader(f, maxSingleFileSize*10+1))
	if err != nil {
		return fmt.Errorf("failed to read zip file")
	}

	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return fmt.Errorf("invalid zip file: %v", err)
	}

	var totalJSONSize int64
	entryCount := 0

	for _, zipEntry := range zipReader.File {
		// 跳过目录
		if zipEntry.FileInfo().IsDir() {
			continue
		}

		// basename 扁平化 + 安全过滤
		basename := path.Base(zipEntry.Name)

		// Zip Slip 防护：拒绝危险路径
		if strings.Contains(zipEntry.Name, "..") ||
			strings.HasPrefix(zipEntry.Name, "/") ||
			len(zipEntry.Name) > 1 && zipEntry.Name[1] == ':' || // Windows 卷名 C:
			basename == "." || basename == "" {
			result.Results = append(result.Results, importResult{
				File:   zipEntry.Name,
				Source: "zip",
				Status: "skipped",
				Error:  "dangerous path in zip",
			})
			result.Skipped++
			continue
		}

		// 仅处理 .json 文件
		if !strings.HasSuffix(strings.ToLower(basename), ".json") {
			result.Results = append(result.Results, importResult{
				File:   zipEntry.Name,
				Source: "zip",
				Status: "skipped",
				Error:  "not a json file",
			})
			result.Skipped++
			continue
		}

		entryCount++
		if entryCount > maxZipEntries {
			result.Results = append(result.Results, importResult{
				File:   zipEntry.Name,
				Source: "zip",
				Status: "skipped",
				Error:  "zip contains too many files (max 50)",
			})
			result.Skipped++
			continue
		}

		// 检查单文件大小
		if zipEntry.UncompressedSize64 > uint64(maxSingleFileSize) {
			result.Results = append(result.Results, importResult{
				File:   basename,
				Source: "zip",
				Status: "failed",
				Error:  "file exceeds 1MB size limit",
			})
			result.Failed++
			continue
		}

		// 检查总 JSON 大小
		totalJSONSize += int64(zipEntry.UncompressedSize64)
		if totalJSONSize > int64(maxZipTotalJSONSize) {
			result.Results = append(result.Results, importResult{
				File:   basename,
				Source: "zip",
				Status: "skipped",
				Error:  "total JSON size exceeds 10MB limit",
			})
			result.Skipped++
			continue
		}

		// basename 去重
		if zipBasenames[basename] {
			result.Results = append(result.Results, importResult{
				File:   basename,
				Source: "zip",
				Status: "duplicate_in_archive",
				Error:  "duplicate basename in archive",
			})
			result.Skipped++
			continue
		}
		zipBasenames[basename] = true

		// 读取文件内容
		rc, err := zipEntry.Open()
		if err != nil {
			result.Results = append(result.Results, importResult{
				File:   basename,
				Source: "zip",
				Status: "failed",
				Error:  "failed to read zip entry",
			})
			result.Failed++
			continue
		}
		data, err := io.ReadAll(io.LimitReader(rc, maxSingleFileSize+1))
		rc.Close()
		if err != nil {
			result.Results = append(result.Results, importResult{
				File:   basename,
				Source: "zip",
				Status: "failed",
				Error:  "failed to read zip entry content",
			})
			result.Failed++
			continue
		}
		if len(data) > maxSingleFileSize {
			result.Results = append(result.Results, importResult{
				File:   basename,
				Source: "zip",
				Status: "failed",
				Error:  "file exceeds 1MB size limit",
			})
			result.Failed++
			continue
		}

		processJSONData(data, basename, "zip", channelID, channel, result, zipBasenames, keyLookup)
	}

	return nil
}

// buildKeyLookup 预构建 account_id/email → keyID 映射，避免每个文件都遍历所有 key
func buildKeyLookup(channel *model.Channel) map[string]int {
	m := make(map[string]int)
	for _, k := range channel.Keys {
		cred, err := auth.ParseCodexCredential(k.ChannelKey)
		if err != nil {
			continue
		}
		if cred.AccountID != "" {
			m["account:"+cred.AccountID] = k.ID
		}
		if cred.Email != "" {
			m["email:"+cred.Email] = k.ID
		}
	}
	return m
}

func processJSONData(data []byte, filename, source string, channelID int, channel *model.Channel, result *importBatchResult, zipBasenames map[string]bool, keyLookup map[string]int) {
	// 解析 JSON
	var authFile codexAuthFile
	if err := json.Unmarshal(data, &authFile); err != nil {
		result.Results = append(result.Results, importResult{
			File:   filename,
			Source: source,
			Status: "failed",
			Error:  "invalid JSON",
		})
		result.Failed++
		return
	}

	// 校验 type == "codex"
	if authFile.Type != "codex" {
		result.Results = append(result.Results, importResult{
			File:   filename,
			Source: source,
			Status: "failed",
			Error:  "type is not codex",
		})
		result.Failed++
		return
	}

	// 校验 refresh_token 必须存在
	if authFile.RefreshToken == "" {
		result.Results = append(result.Results, importResult{
			File:   filename,
			Source: source,
			Status: "failed",
			Error:  "missing refresh_token",
		})
		result.Failed++
		return
	}

	// 解析 id_token JWT 提取 email 和 account_id
	var jwtEmail, jwtAccountID string
	if authFile.IDToken != "" {
		jwtEmail, jwtAccountID = auth.ParseIDTokenPayload(authFile.IDToken)
	}

	// 合并 email：JSON email > JWT email > account_id > 文件名
	email := authFile.Email
	if email == "" {
		email = jwtEmail
	}
	if email == "" && authFile.AccountID != "" {
		email = authFile.AccountID
	}
	if email == "" {
		// 使用文件名（去掉 .json 后缀）
		email = strings.TrimSuffix(filename, ".json")
	}

	// 合并 account_id：JSON account_id > JWT chatgpt_account_id
	accountID := authFile.AccountID
	if accountID == "" {
		accountID = jwtAccountID
	}

	// 解析 expired 字段（支持 RFC3339 和 Unix 时间戳）
	var expiresAt string
	if len(authFile.Expired) > 0 {
		var expiredStr string
		// 尝试作为字符串解析
		if err := json.Unmarshal(authFile.Expired, &expiredStr); err == nil {
			// 尝试 RFC3339
			if t, err := auth.ParseExpiresAt(expiredStr); err == nil {
				expiresAt = t.Format("2006-01-02T15:04:05Z07:00")
			}
		} else {
			// 尝试作为数字（Unix 时间戳）
			var expiredNum float64
			if err := json.Unmarshal(authFile.Expired, &expiredNum); err == nil {
				expiresAt = fmt.Sprintf("%.0f", expiredNum)
				if t, err := auth.ParseExpiresAt(expiresAt); err == nil {
					expiresAt = t.Format("2006-01-02T15:04:05Z07:00")
				}
			}
		}
	}

	// 判断是否 incomplete（缺少 access_token 或 id_token）
	isIncomplete := authFile.AccessToken == "" || authFile.IDToken == ""

	// 构建 remark：只存邮箱标识
	remark := email

	// 构建 CodexCredential JSON（与 OAuth 流程格式一致）
	cred := auth.CodexCredential{
		AccessToken:  authFile.AccessToken,
		RefreshToken: authFile.RefreshToken,
		IDToken:      authFile.IDToken,
		ExpiresAt:    expiresAt,
		AccountID:    accountID,
		Email:        email,
		TokenType:    "Bearer",
	}
	credJSON := cred.String()

	// Upsert：查找现有 key 是否匹配 account_id 或 email
	var existingKeyID int
	if keyLookup != nil {
		if accountID != "" {
			if kid, ok := keyLookup["account:"+accountID]; ok {
				existingKeyID = kid
			}
		}
		if existingKeyID == 0 && email != "" {
			if kid, ok := keyLookup["email:"+email]; ok {
				existingKeyID = kid
			}
		}
	}

	if existingKeyID > 0 {
		// 更新现有 key
		updateReq := &model.ChannelUpdateRequest{
			ID: channelID,
			KeysToUpdate: []model.ChannelKeyUpdateRequest{
				{
					ID:         existingKeyID,
					ChannelKey: &credJSON,
					IsCLI:      ptrBool(true),
					Remark:     &remark,
				},
			},
		}
		updated, err := op.ChannelUpdate(updateReq, context.Background())
		if err != nil {
			result.Results = append(result.Results, importResult{
				File:      filename,
				Source:    source,
				Status:    "failed",
				Email:     email,
				AccountID: accountID,
				Error:     "failed to update channel key",
			})
			result.Failed++
			return
		}
		*channel = *updated // 刷新 channel 对象，后续导入能看到最新的 keys
		// 更新 keyLookup 防止同批次重复
		if accountID != "" {
			keyLookup["account:"+accountID] = existingKeyID
		}
		if email != "" {
			keyLookup["email:"+email] = existingKeyID
		}
		status := "updated"
		if isIncomplete {
			status = "incomplete"
		}
		result.Results = append(result.Results, importResult{
			File:      filename,
			Source:    source,
			Status:    status,
			Email:     email,
			AccountID: accountID,
		})
		result.Updated++
	} else {
		// 新增 key
		addReq := &model.ChannelUpdateRequest{
			ID: channelID,
			KeysToAdd: []model.ChannelKeyAddRequest{
				{
					Enabled:    !authFile.Disabled,
					ChannelKey: credJSON,
					IsCLI:      true,
					Multiplier: 1,
					Remark:     remark,
				},
			},
		}
		updated, err := op.ChannelUpdate(addReq, context.Background())
		if err != nil {
			result.Results = append(result.Results, importResult{
				File:      filename,
				Source:    source,
				Status:    "failed",
				Email:     email,
				AccountID: accountID,
				Error:     "failed to add channel key",
			})
			result.Failed++
			return
		}
		*channel = *updated // 刷新 channel 对象，后续导入能看到最新的 keys
		// 更新 keyLookup 防止同批次重复
		for _, k := range updated.Keys {
			if k.ID != 0 && k.ChannelID == channelID {
				if k.Remark != "" {
					keyLookup["email:"+k.Remark] = k.ID
				}
				c, err := auth.ParseCodexCredential(k.ChannelKey)
				if err == nil && c.AccountID != "" {
					keyLookup["account:"+c.AccountID] = k.ID
				}
			}
		}
		status := "imported"
		if isIncomplete {
			status = "incomplete"
		}
		result.Results = append(result.Results, importResult{
			File:      filename,
			Source:    source,
			Status:    status,
			Email:     email,
			AccountID: accountID,
		})
		result.Imported++
	}
}

func ptrBool(v bool) *bool {
	return &v
}
