package im

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWeChatAdapterSendFileMatchesOfficialPayload(t *testing.T) {
	t.Parallel()

	var (
		mu             sync.Mutex
		handlerErr     error
		getUploadReq   map[string]any
		sendMessageReq map[string]any
		uploadCounts   = make(map[string]int)
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ilink/bot/getuploadurl":
			defer r.Body.Close()
			payload := make(map[string]any)
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				mu.Lock()
				handlerErr = err
				mu.Unlock()
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			getUploadReq = payload
			mu.Unlock()
			writeJSON(t, w, map[string]any{
				"upload_param": "file-param",
			})
		case "/cdn/upload":
			kind := r.URL.Query().Get("encrypted_query_param")
			mu.Lock()
			uploadCounts[kind]++
			mu.Unlock()
			if kind == "" {
				http.Error(w, "missing encrypted_query_param", http.StatusBadRequest)
				mu.Lock()
				handlerErr = errMissingQueryParam
				mu.Unlock()
				return
			}
			w.Header().Set("x-encrypted-param", "download-"+kind)
			w.WriteHeader(http.StatusOK)
		case "/ilink/bot/sendmessage":
			defer r.Body.Close()
			payload := make(map[string]any)
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				mu.Lock()
				handlerErr = err
				mu.Unlock()
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			sendMessageReq = payload
			mu.Unlock()
			writeJSON(t, w, map[string]any{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	filePath := filepath.Join(t.TempDir(), "story.txt")
	content := []byte("hello from wechat file delivery")
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	adapter := &WeChatAdapter{
		client:     server.Client(),
		cdnBaseURL: server.URL + "/cdn",
		logins:     make(map[string]*activeWeChatLogin),
	}

	_, err := adapter.SendFile(context.Background(), Account{
		ID:       "acc_1",
		Platform: PlatformWeChat,
		BaseURL:  server.URL,
		Token:    "token",
	}, Target{
		ID:           "target_1",
		AccountID:    "acc_1",
		TargetUserID: "user@im.wechat",
	}, PreparedFile{
		FilePath: filePath,
		FileName: filepath.Base(filePath),
		MimeType: "text/plain",
		Size:     int64(len(content)),
	}, "")
	if err != nil {
		t.Fatalf("SendFile() error = %v", err)
	}

	mu.Lock()
	if handlerErr != nil {
		t.Fatalf("handler error = %v", handlerErr)
	}
	if getUploadReq == nil {
		mu.Unlock()
		t.Fatal("getuploadurl request was not captured")
	}
	if got := int(getUploadReq["media_type"].(float64)); got != weChatUploadMediaTypeFile {
		mu.Unlock()
		t.Fatalf("media_type = %d, want %d", got, weChatUploadMediaTypeFile)
	}
	if got := getUploadReq["no_need_thumb"].(bool); !got {
		mu.Unlock()
		t.Fatal("no_need_thumb = false, want true")
	}
	if _, ok := getUploadReq["thumb_rawsize"]; ok {
		mu.Unlock()
		t.Fatal("thumb_rawsize exists, want omitted")
	}
	if _, ok := getUploadReq["thumb_filesize"]; ok {
		mu.Unlock()
		t.Fatal("thumb_filesize exists, want omitted")
	}
	if _, ok := getUploadReq["thumb_rawfilemd5"]; ok {
		mu.Unlock()
		t.Fatal("thumb_rawfilemd5 exists, want omitted")
	}

	origUploads := uploadCounts["file-param"]
	if sendMessageReq == nil {
		mu.Unlock()
		t.Fatal("sendmessage request was not captured")
	}
	sendReq := sendMessageReq
	mu.Unlock()

	if origUploads != 1 {
		t.Fatalf("orig upload count = %d, want 1", origUploads)
	}

	msg := sendReq["msg"].(map[string]any)
	itemList := msg["item_list"].([]any)
	if len(itemList) != 1 {
		t.Fatalf("len(item_list) = %d, want 1", len(itemList))
	}
	fileItem := itemList[0].(map[string]any)["file_item"].(map[string]any)
	if got := fileItem["file_name"].(string); got != "story.txt" {
		t.Fatalf("file_name = %q, want %q", got, "story.txt")
	}
	if got := fileItem["len"].(string); got != fmt.Sprintf("%d", len(content)) {
		t.Fatalf("len = %q, want %q", got, fmt.Sprintf("%d", len(content)))
	}

	media := fileItem["media"].(map[string]any)
	if got := media["encrypt_query_param"].(string); got != "download-file-param" {
		t.Fatalf("media.encrypt_query_param = %q, want %q", got, "download-file-param")
	}
	if got := media["aes_key"].(string); got == "" {
		t.Fatal("media.aes_key = empty, want encoded key")
	}
}
