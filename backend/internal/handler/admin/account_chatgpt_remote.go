package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

type chatGPTRemoteImportRequest struct {
	chatGPTImportRequest
	Kind    string `json:"kind"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
}

// Remote credentials are used only for this request. Connection-level address
// validation and redirect rejection keep the management fetch within its target.
func (h *AccountHandler) ImportRemoteChatGPTAccounts(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var req chatGPTRemoteImportRequest
	if c.ShouldBindJSON(&req) != nil || (req.Kind != "cpa" && req.Kind != "sub2api") || strings.TrimSpace(req.APIKey) == "" {
		response.BadRequest(c, "provide remote server kind, URL and API key")
		return
	}
	base, err := url.Parse(strings.TrimRight(strings.TrimSpace(req.BaseURL), "/"))
	if err != nil || base.Scheme != "https" || base.Hostname() == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		response.BadRequest(c, "remote server must be an HTTPS base URL without credentials or query")
		return
	}
	if req.Source == "" {
		req.Source = "codex"
	}
	if req.Source != "codex" && req.Source != "web" {
		response.BadRequest(c, "invalid account source")
		return
	}
	client, err := httpclient.GetClient(httpclient.Options{Timeout: 30 * time.Second, ValidateResolvedIP: true})
	if err != nil {
		response.InternalError(c, "cannot configure remote connection")
		return
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	ctx, cancel := context.WithTimeout(c.Request.Context(), 100*time.Second)
	defer cancel()
	var bytesRead atomic.Int64
	fetch := func(ctx context.Context, path string, target any) error {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String()+path, nil)
		if err != nil {
			return errors.New("invalid remote request")
		}
		request.Header.Set("Authorization", "Bearer "+req.APIKey)
		request.Header.Set("Accept", "application/json")
		res, err := copyClient.Do(request)
		if err != nil {
			return errors.New("remote server is unavailable or its address is not allowed")
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("remote server returned HTTP %d", res.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(res.Body, (12<<20)+1))
		if err != nil || len(data) > 12<<20 {
			return errors.New("remote response exceeds 12 MiB")
		}
		if bytesRead.Add(int64(len(data))) > 12<<20 {
			return errors.New("total remote import exceeds 12 MiB")
		}
		if json.Unmarshal(data, target) != nil {
			return errors.New("remote server returned invalid JSON")
		}
		return nil
	}
	var values []any
	if req.Kind == "cpa" {
		var list struct {
			Files []struct {
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"files"`
		}
		if err = fetch(ctx, "/v0/management/auth-files", &list); err == nil {
			if len(list.Files) > 1000 {
				response.BadRequest(c, "remote pool exceeds 1000 files; use file export instead")
				return
			}
			values = make([]any, len(list.Files))
			g, gctx := errgroup.WithContext(ctx)
			g.SetLimit(5)
			for index, file := range list.Files {
				g.Go(func() error {
					if file.Type != "" && file.Type != "codex" && file.Type != "openai" {
						return nil
					}
					var payload any
					if err := fetch(gctx, "/v0/management/auth-files/download?name="+url.QueryEscape(file.Name), &payload); err != nil {
						return err
					}
					values[index] = payload
					return nil
				})
			}
			err = g.Wait()
		}
	} else {
		// Current Sub2API protects raw exports with step-up authentication. A
		// remote server may consequently reject API-key exports; surface that
		// response and let the operator use its verified JSON export instead.
		var envelope struct {
			Code int `json:"code"`
			Data struct {
				Accounts []any `json:"accounts"`
			} `json:"data"`
		}
		err = fetch(ctx, "/api/v1/admin/accounts/data?platform=openai&include_proxies=false", &envelope)
		if err == nil && envelope.Code != 0 {
			err = errors.New("remote export was rejected; use a verified JSON export")
		}
		values = envelope.Data.Accounts
	}
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	filtered := make([]any, 0, len(values))
	for _, value := range values {
		if value != nil {
			filtered = append(filtered, value)
		}
	}
	values = chatGPTImportValues(filtered)
	if len(values) == 0 || len(values) > 1000 {
		response.BadRequest(c, "no importable accounts found, or more than 1000 accounts returned")
		return
	}
	result, err := h.importChatGPTAccounts(ctx, req.chatGPTImportRequest, values)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
