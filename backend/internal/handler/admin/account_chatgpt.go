package admin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

type chatGPTImportRequest struct {
	Content        string   `json:"content"`
	Contents       []string `json:"contents"`
	Name           string   `json:"name"`
	Source         string   `json:"source"`
	GroupIDs       []int64  `json:"group_ids"`
	ProxyID        *int64   `json:"proxy_id"`
	UpdateExisting bool     `json:"update_existing"`
}

func (h *AccountHandler) ListChatGPTAccounts(c *gin.Context) {
	manager, ok := h.adminService.(service.ChatGPTAccountManager)
	if !ok {
		response.Error(c, http.StatusServiceUnavailable, "ChatGPT account pool is unavailable")
		return
	}
	var groupID int64
	if group := c.Query("group"); group == "ungrouped" {
		groupID = service.AccountListGroupUngrouped
	} else if group != "" {
		var err error
		groupID, err = strconv.ParseInt(group, 10, 64)
		if err != nil || groupID <= 0 {
			response.BadRequest(c, "invalid group")
			return
		}
	}
	accounts, err := manager.ListChatGPTAccountPool(c.Request.Context(), groupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	search, source, status := strings.ToLower(strings.TrimSpace(c.Query("search"))), c.Query("source"), c.Query("status")
	filtered := make([]*dto.AccountListItem, 0, len(accounts))
	counts := map[string]int{}
	for i := range accounts {
		a := &accounts[i]
		if source != "" && a.GetCredential("source_type") != source {
			continue
		}
		if search != "" {
			fields := []string{a.Name, a.GetCredential("email"), a.GetCredential("chatgpt_user_id"), a.GetCredential("chatgpt_account_id"), a.GetCredential("plan_type"), a.GetCredential("source_type")}
			if a.Notes != nil {
				fields = append(fields, *a.Notes)
			}
			if !strings.Contains(strings.ToLower(strings.Join(fields, " ")), search) {
				continue
			}
		}
		state := service.ChatGPTAccountPoolState(a, time.Now())
		counts[state.Status]++
		if status != "" && state.Status != status {
			continue
		}
		filtered = append(filtered, dto.AccountListItemFromAccount(dto.AccountFromServiceShallow(a)))
	}
	if c.Query("ids_only") == "1" {
		ids := make([]int64, 0, len(filtered))
		for _, a := range filtered {
			ids = append(ids, a.ID)
		}
		response.Success(c, gin.H{"ids": ids})
		return
	}
	page, size := response.ParsePagination(c)
	if size > 1000 {
		size = 1000
	}
	total := len(filtered)
	start := min((page-1)*size, total)
	end := min(start+size, total)
	pages := (total + size - 1) / size
	response.Success(c, gin.H{"items": filtered[start:end], "total": total, "page": page, "page_size": size, "pages": pages, "counts": counts})
}

func chatGPTImportValues(values []any) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		if object, ok := value.(map[string]any); ok {
			if firstCodexString(object, []string{"access_token"}, []string{"accessToken"}, []string{"tokens", "access_token"}, []string{"credentials", "access_token"}) == "" {
				unwrapped := false
				for _, key := range []string{"accounts", "items", "results", "data"} {
					if rows, ok := object[key].([]any); ok {
						out = append(out, chatGPTImportValues(flattenCodexImportValues(rows))...)
						unwrapped = true
						break
					}
					if data, ok := object[key].(map[string]any); ok {
						out = append(out, chatGPTImportValues([]any{data})...)
						unwrapped = true
						break
					}
				}
				if unwrapped {
					continue
				}
			}
			// Account archives may carry credentials in several established formats.
			for _, key := range []string{"credentials", "credential", "auth"} {
				if credentials, ok := object[key].(map[string]any); ok {
					copy := make(map[string]any, len(object)+len(credentials))
					for k, v := range object {
						copy[k] = v
					}
					for k, v := range credentials {
						if _, exists := copy[k]; !exists {
							copy[k] = v
						}
					}
					object = copy
				}
			}
			out = append(out, object)
		} else {
			out = append(out, value)
		}
	}
	return out
}

func (h *AccountHandler) ImportChatGPTAccounts(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<20)
	var req chatGPTImportRequest
	if c.ShouldBindJSON(&req) != nil {
		response.BadRequest(c, "invalid import request (maximum 16 MiB)")
		return
	}
	if req.Source == "" {
		req.Source = "web"
	}
	if req.Source != "web" && req.Source != "codex" {
		response.BadRequest(c, "source must be web or codex")
		return
	}
	contents := append([]string{req.Content}, req.Contents...)
	var values []any
	for _, content := range contents {
		rows, err := parseCodexSessionImportContent(content)
		if err != nil {
			response.BadRequest(c, "invalid JSON or token input")
			return
		}
		values = append(values, chatGPTImportValues(rows)...)
		if len(values) > 1000 {
			response.BadRequest(c, "import at most 1000 accounts at a time")
			return
		}
	}
	if len(values) == 0 {
		response.BadRequest(c, "no account credentials were provided")
		return
	}
	if err := h.adminService.ValidateAccountGroupBindings(c.Request.Context(), req.GroupIDs); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	executeAdminIdempotentJSON(c, "admin.accounts.import_chatgpt", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.importChatGPTAccounts(ctx, req, values)
	})
}

func chatGPTIdentity(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func (h *AccountHandler) importChatGPTAccounts(ctx context.Context, req chatGPTImportRequest, values []any) (CodexSessionImportResult, error) {
	result := CodexSessionImportResult{Total: len(values)}
	byIdentity := make(map[string]int64)
	for page := 1; ; page++ {
		accounts, total, err := h.adminService.ListAccounts(ctx, page, 1000, service.PlatformChatGPT, "", "", "", 0, "", "id", "asc")
		if err != nil {
			return result, err
		}
		for _, a := range accounts {
			byIdentity["at:"+chatGPTIdentity(a.GetCredential("access_token"))] = a.ID
			if rt := a.GetCredential("refresh_token"); rt != "" {
				byIdentity["rt:"+chatGPTIdentity(rt)] = a.ID
			}
		}
		if int64(page*1000) >= total || len(accounts) == 0 {
			break
		}
	}
	for index, value := range values {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		item, err := normalizeCodexImportEntryAllowExpired(codexImportEntry{Index: index + 1, Value: value}, true)
		if err != nil || item.IsAgentIdentity {
			result.Failed++
			result.Errors = append(result.Errors, CodexSessionImportMessage{Index: index + 1, Message: "missing or invalid account token"})
			continue
		}
		name := item.Name
		if req.Name != "" {
			name = req.Name
			if len(values) > 1 {
				name = fmt.Sprintf("%s-%d", name, index+1)
			}
		}
		if len([]rune(name)) > 100 {
			name = string([]rune(name)[:100])
		}
		source := req.Source
		enabled := true
		var notes *string
		clientID := ""
		if raw, ok := value.(map[string]any); ok {
			clientID = firstCodexString(raw, []string{"client_id"})
			if text, ok := raw["notes"].(string); ok {
				notes = &text
			}
			if item.PlanType == "" {
				item.PlanType = firstCodexString(raw, []string{"plan"}, []string{"type"})
				if item.PlanType == "oauth" || item.PlanType == "codex" {
					item.PlanType = ""
				}
				if item.PlanType != "" {
					item.Credentials["plan_type"] = item.PlanType
				}
			}
			if rawSource := firstCodexString(raw, []string{"source_type"}, []string{"source"}); rawSource == "web" || rawSource == "codex" {
				source = rawSource
			}
			if rawEnabled, ok := raw["enabled"].(bool); ok {
				enabled = rawEnabled
			}
			if raw["status"] == "inactive" {
				enabled = false
			}
		}
		item.Credentials["source_type"] = source
		if clientID == "" {
			clientID = "app_2SKx67EdpoN0G6j64rFvigXD"
			if source == "codex" {
				clientID = openai.ClientID
			}
		}
		item.Credentials["client_id"] = clientID
		extra := map[string]any{"import_source": "chatgpt_account_pool", "chatgpt_access_invalid": false, "chatgpt_refresh_invalid": false}
		id := byIdentity["at:"+chatGPTIdentity(item.AccessToken)]
		if id == 0 && item.RefreshToken != "" {
			id = byIdentity["rt:"+chatGPTIdentity(item.RefreshToken)]
		}
		if id != 0 && !req.UpdateExisting {
			result.Skipped++
			result.Items = append(result.Items, CodexSessionImportItem{Index: index + 1, Name: name, Action: "skipped", AccountID: id})
			continue
		}
		var saved *service.Account
		if id != 0 {
			// Import only changes supplied credentials; preserve per-account settings
			// and the previous quota observation until the next successful sync.
			current, getErr := h.adminService.GetAccount(ctx, id)
			if getErr != nil {
				err = getErr
			} else {
				credentials := make(map[string]any, len(current.Credentials)+len(item.Credentials))
				for k, v := range current.Credentials {
					credentials[k] = v
				}
				for k, v := range item.Credentials {
					credentials[k] = v
				}
				input := &service.UpdateAccountInput{Name: name, Notes: notes, Credentials: credentials, Extra: extra, ProxyID: req.ProxyID}
				if req.GroupIDs != nil {
					input.GroupIDs = &req.GroupIDs
				}
				saved, err = h.adminService.UpdateAccount(ctx, id, input)
			}
		} else {
			saved, err = h.adminService.CreateAccount(ctx, &service.CreateAccountInput{Name: name, Notes: notes, Platform: service.PlatformChatGPT, Type: service.AccountTypeOAuth, Credentials: item.Credentials, Extra: extra, ProxyID: req.ProxyID, GroupIDs: req.GroupIDs, Concurrency: 1, Priority: 1, SkipDefaultGroupBind: true})
			if err == nil && !enabled {
				saved, err = h.adminService.UpdateAccount(ctx, saved.ID, &service.UpdateAccountInput{Status: "inactive"})
			}
		}
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, CodexSessionImportMessage{Index: index + 1, Name: name, Message: "could not save account; check group and proxy configuration"})
			continue
		}
		action := "created"
		if id != 0 {
			action = "updated"
			result.Updated++
		} else {
			result.Created++
		}
		byIdentity["at:"+chatGPTIdentity(item.AccessToken)] = saved.ID
		if item.RefreshToken != "" {
			byIdentity["rt:"+chatGPTIdentity(item.RefreshToken)] = saved.ID
		}
		result.Items = append(result.Items, CodexSessionImportItem{Index: index + 1, Name: name, Action: action, AccountID: saved.ID})
		if item.RefreshToken == "" {
			result.Warnings = append(result.Warnings, CodexSessionImportMessage{Index: index + 1, Name: name, Message: "no refresh token; access token cannot be renewed automatically"})
		}
	}
	return result, nil
}

func (h *AccountHandler) MaintainChatGPTAccounts(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var req struct {
		AccountIDs []int64 `json:"account_ids"`
		Action     string  `json:"action"`
	}
	if c.ShouldBindJSON(&req) != nil || len(req.AccountIDs) == 0 || len(req.AccountIDs) > 1000 || (req.Action != "sync" && req.Action != "refresh") {
		response.BadRequest(c, "provide 1 to 1000 account_ids and action sync or refresh")
		return
	}
	for _, id := range req.AccountIDs {
		if id <= 0 {
			response.BadRequest(c, "account_ids must contain positive IDs")
			return
		}
	}
	manager, ok := h.adminService.(service.ChatGPTAccountManager)
	if !ok {
		response.Error(c, http.StatusServiceUnavailable, "ChatGPT account maintenance is unavailable")
		return
	}
	type operationItem struct {
		AccountID int64        `json:"account_id"`
		Success   bool         `json:"success"`
		Error     string       `json:"error,omitempty"`
		Account   *dto.Account `json:"account,omitempty"`
	}
	items := make([]operationItem, 0, len(req.AccountIDs))
	seen := make(map[int64]bool)
	var mu sync.Mutex
	g, ctx := errgroup.WithContext(c.Request.Context())
	g.SetLimit(5)
	for _, id := range req.AccountIDs {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		g.Go(func() error {
			account, err := manager.MaintainChatGPTAccount(ctx, id, req.Action)
			item := operationItem{AccountID: id, Success: err == nil}
			if err != nil {
				item.Error = err.Error()
			} else {
				item.Account = dto.AccountFromService(account)
			}
			mu.Lock()
			items = append(items, item)
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()
	success := 0
	for _, item := range items {
		if item.Success {
			success++
		}
	}
	response.Success(c, gin.H{"total": len(items), "success": success, "failed": len(items) - success, "items": items})
}

func (h *AccountHandler) GetChatGPTCredential(c *gin.Context) {
	id, valid := parsePositiveIDParam(c, "id")
	if !valid {
		return
	}
	key := c.Param("kind")
	if key != "access_token" && key != "refresh_token" {
		response.BadRequest(c, "unsupported credential kind")
		return
	}
	a, err := h.adminService.GetAccount(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if a.Platform != service.PlatformChatGPT {
		response.BadRequest(c, "a ChatGPT account is required")
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"token": a.GetCredential(key)})
}
