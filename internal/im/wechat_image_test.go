package im

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWeChatAdapterSendImageMatchesOfficialPayload(t *testing.T) {
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
				"upload_param": "orig-param",
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

	imagePath, imageBytes := writeTestPNG(t, 800, 600)
	adapter := &WeChatAdapter{
		client:     server.Client(),
		cdnBaseURL: server.URL + "/cdn",
		logins:     make(map[string]*activeWeChatLogin),
	}

	_, err := adapter.SendImage(context.Background(), Account{
		ID:       "acc_1",
		Platform: PlatformWeChat,
		BaseURL:  server.URL,
		Token:    "token",
	}, Target{
		ID:           "target_1",
		AccountID:    "acc_1",
		TargetUserID: "user@im.wechat",
	}, PreparedImage{
		FilePath: imagePath,
		FileName: filepath.Base(imagePath),
		MimeType: "image/png",
		Size:     int64(len(imageBytes)),
	}, "")
	if err != nil {
		t.Fatalf("SendImage() error = %v", err)
	}
	mu.Lock()
	if handlerErr != nil {
		t.Fatalf("handler error = %v", handlerErr)
	}
	mu.Unlock()

	mu.Lock()
	if getUploadReq == nil {
		mu.Unlock()
		t.Fatal("getuploadurl request was not captured")
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

	origUploads := uploadCounts["orig-param"]
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
	imageItem := itemList[0].(map[string]any)["image_item"].(map[string]any)
	if got := int(imageItem["mid_size"].(float64)); got <= 0 {
		t.Fatalf("mid_size = %d, want > 0", got)
	}
	if _, ok := imageItem["thumb_media"]; ok {
		t.Fatal("thumb_media exists, want omitted")
	}
	if _, ok := imageItem["thumb_size"]; ok {
		t.Fatal("thumb_size exists, want omitted")
	}
	if _, ok := imageItem["thumb_width"]; ok {
		t.Fatal("thumb_width exists, want omitted")
	}
	if _, ok := imageItem["thumb_height"]; ok {
		t.Fatal("thumb_height exists, want omitted")
	}
	if _, ok := imageItem["aeskey"]; ok {
		t.Fatal("image_item.aeskey exists, want omitted")
	}

	media := imageItem["media"].(map[string]any)
	if got := media["encrypt_query_param"].(string); got != "download-orig-param" {
		t.Fatalf("media.encrypt_query_param = %q, want %q", got, "download-orig-param")
	}
	if got := media["aes_key"].(string); got == "" {
		t.Fatal("media.aes_key = empty, want encoded key")
	}
}

func TestWeChatAdapterSendImageIgnoresThumbUploadParamWhenServerReturnsIt(t *testing.T) {
	t.Parallel()

	var (
		mu             sync.Mutex
		handlerErr     error
		sendMessageReq map[string]any
		uploadCounts   = make(map[string]int)
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ilink/bot/getuploadurl":
			writeJSON(t, w, map[string]any{
				"upload_param":       "orig-param",
				"thumb_upload_param": "thumb-param",
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

	imagePath, imageBytes := writeTestPNG(t, 800, 600)
	adapter := &WeChatAdapter{
		client:     server.Client(),
		cdnBaseURL: server.URL + "/cdn",
		logins:     make(map[string]*activeWeChatLogin),
	}

	_, err := adapter.SendImage(context.Background(), Account{
		ID:       "acc_1",
		Platform: PlatformWeChat,
		BaseURL:  server.URL,
		Token:    "token",
	}, Target{
		ID:           "target_1",
		AccountID:    "acc_1",
		TargetUserID: "user@im.wechat",
	}, PreparedImage{
		FilePath: imagePath,
		FileName: filepath.Base(imagePath),
		MimeType: "image/png",
		Size:     int64(len(imageBytes)),
	}, "")
	if err != nil {
		t.Fatalf("SendImage() error = %v", err)
	}

	mu.Lock()
	if handlerErr != nil {
		mu.Unlock()
		t.Fatalf("handler error = %v", handlerErr)
	}
	origUploads := uploadCounts["orig-param"]
	if sendMessageReq == nil {
		mu.Unlock()
		t.Fatal("sendmessage request was not captured")
	}
	sendReq := sendMessageReq
	mu.Unlock()

	if origUploads != 1 {
		t.Fatalf("orig upload count = %d, want 1", origUploads)
	}
	if got := uploadCounts["thumb-param"]; got != 0 {
		t.Fatalf("thumb upload count = %d, want 0", got)
	}
	msg := sendReq["msg"].(map[string]any)
	itemList := msg["item_list"].([]any)
	imageItem := itemList[0].(map[string]any)["image_item"].(map[string]any)
	if _, ok := imageItem["thumb_media"]; ok {
		t.Fatal("thumb_media exists, want omitted in official payload")
	}
	if _, ok := imageItem["thumb_size"]; ok {
		t.Fatal("thumb_size exists, want omitted in official payload")
	}
	if _, ok := imageItem["thumb_width"]; ok {
		t.Fatal("thumb_width exists, want omitted in official payload")
	}
	if _, ok := imageItem["thumb_height"]; ok {
		t.Fatal("thumb_height exists, want omitted in official payload")
	}
}

var errMissingQueryParam = &requestError{message: "missing encrypted_query_param"}

type requestError struct {
	message string
}

func (e *requestError) Error() string {
	return e.message
}

func writeJSON(t *testing.T, w http.ResponseWriter, payload map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode json response: %v", err)
	}
}

func writeTestPNG(t *testing.T, width int, height int) (string, []byte) {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x * 255) / width),
				G: uint8((y * 255) / height),
				B: uint8((x + y) % 255),
				A: 255,
			})
		}
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("os.Create() error = %v", err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		t.Fatalf("png.Encode() error = %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	return path, content
}
