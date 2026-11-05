package httpapi

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/jmoiron/sqlx"

	"resumepro/backend/internal/speech"
)

// wavBytes 生成一个最小的合法 16kHz 单声道 WAV。
func wavBytes(samples int) []byte {
	dataSize := samples * 2
	buf := &bytes.Buffer{}
	_ = binary.Write(buf, binary.LittleEndian, []byte("RIFF"))
	_ = binary.Write(buf, binary.LittleEndian, uint32(36+dataSize))
	_ = binary.Write(buf, binary.LittleEndian, []byte("WAVE"))
	_ = binary.Write(buf, binary.LittleEndian, []byte("fmt "))
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(buf, binary.LittleEndian, uint32(16000))
	_ = binary.Write(buf, binary.LittleEndian, uint32(32000))
	_ = binary.Write(buf, binary.LittleEndian, uint16(2))
	_ = binary.Write(buf, binary.LittleEndian, uint16(16))
	_ = binary.Write(buf, binary.LittleEndian, []byte("data"))
	_ = binary.Write(buf, binary.LittleEndian, uint32(dataSize))
	buf.Write(make([]byte, dataSize))
	return buf.Bytes()
}

// stubTranscriber 返回固定文本。
type stubTranscriber struct {
	text string
}

func (s *stubTranscriber) Transcribe(_ context.Context, _ speech.TranscriptionRequest) (speech.TranscriptionResult, error) {
	return speech.TranscriptionResult{Text: s.text}, nil
}

// setupVoiceAPI 复用既有面试路由测试的装配（内存 SQLite + miniredis + stub 模型），
// 额外注入一个语音服务（临时目录 AudioStore + stub transcriber）。
func setupVoiceAPI(t *testing.T) (http.Handler, *http.Cookie, *sqlx.DB, *speech.AudioStore) {
	t.Helper()
	store := speech.NewAudioStore(t.TempDir())
	voice := speech.NewVoiceService(store, &stubTranscriber{text: "识别出来的文字"}, nil)
	router, cookie, db := setupInterviewAPI(t, voice)
	return router, cookie, db, store
}

// firstQuestionID 读回面试第一道题的 id。
func firstQuestionID(t *testing.T, router http.Handler, cookie *http.Cookie, interviewID int64) int64 {
	t.Helper()
	response := requestJSON(t, router, http.MethodGet, "/api/interviews/"+jsonNumber(interviewID)+"/questions", "", cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("questions: %d %s", response.Code, response.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode questions: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("no questions")
	}
	id, ok := rows[0]["id"].(float64)
	if !ok {
		t.Fatalf("question id = %v", rows[0]["id"])
	}
	return int64(id)
}

// TestVoiceAudioReturns404ForUnknownInterview 面试不存在 → 404。
func TestVoiceAudioReturns404ForUnknownInterview(t *testing.T) {
	router, cookie, _, _ := setupVoiceAPI(t)
	response := requestJSON(t, router, http.MethodGet, "/api/interviews/999/questions/1/audio", "", cookie)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
}

// TestVoiceRoutesRegisteredWithoutVoiceService 语音未启用时路由仍然存在，返回 503（不是 404）。
func TestVoiceRoutesRegisteredWithoutVoiceService(t *testing.T) {
	router, cookie, _ := setupInterviewAPI(t)
	audio := requestJSON(t, router, http.MethodGet, "/api/interviews/1/questions/1/audio", "", cookie)
	if audio.Code != http.StatusServiceUnavailable {
		t.Fatalf("audio status = %d, want 503", audio.Code)
	}
	transcribe := requestJSON(t, router, http.MethodPost, "/api/interviews/1/questions/1/transcribe", "", cookie)
	if transcribe.Code != http.StatusServiceUnavailable {
		t.Fatalf("transcribe status = %d, want 503", transcribe.Code)
	}
}

// TestVoiceAudioReturnsConflictWhenNotReady 语音未就绪 → 409（前端显示生成中）。
func TestVoiceAudioReturnsConflictWhenNotReady(t *testing.T) {
	router, cookie, _, _ := setupVoiceAPI(t)
	_, interviewID := createInterviewFixture(t, router, cookie, nil)
	questionID := firstQuestionID(t, router, cookie, interviewID)

	response := requestJSON(t, router, http.MethodGet,
		"/api/interviews/"+jsonNumber(interviewID)+"/questions/"+jsonNumber(questionID)+"/audio", "", cookie)
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", response.Code)
	}
}

// TestVoiceAudioReturnsWavWhenReady 语音就绪 → 200 audio/wav。
func TestVoiceAudioReturnsWavWhenReady(t *testing.T) {
	router, cookie, db, store := setupVoiceAPI(t)
	_, interviewID := createInterviewFixture(t, router, cookie, nil)
	questionID := firstQuestionID(t, router, cookie, interviewID)

	audio := wavBytes(16000)
	if err := store.SaveQuestionAudio(context.Background(), interviewTenantID(t, db, interviewID), interviewID, questionID, audio); err != nil {
		t.Fatalf("save question audio: %v", err)
	}
	markAudioState(t, db, questionID, "ready")

	request := httptest.NewRequest(http.MethodGet,
		"/api/interviews/"+jsonNumber(interviewID)+"/questions/"+jsonNumber(questionID)+"/audio", nil)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "audio/wav" {
		t.Errorf("content type = %q, want audio/wav", got)
	}
	if !bytes.Equal(recorder.Body.Bytes(), audio) {
		t.Errorf("body length = %d, want %d", recorder.Body.Len(), len(audio))
	}
}

// TestVoiceAudioReturns404WhenFileMissing 状态就绪但文件缺失 → 404。
func TestVoiceAudioReturns404WhenFileMissing(t *testing.T) {
	router, cookie, db, _ := setupVoiceAPI(t)
	_, interviewID := createInterviewFixture(t, router, cookie, nil)
	questionID := firstQuestionID(t, router, cookie, interviewID)
	markAudioState(t, db, questionID, "ready")

	response := requestJSON(t, router, http.MethodGet,
		"/api/interviews/"+jsonNumber(interviewID)+"/questions/"+jsonNumber(questionID)+"/audio", "", cookie)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
}

// TestVoiceTranscribeRejectsInvalidAudio 非法音频 → 400。
func TestVoiceTranscribeRejectsInvalidAudio(t *testing.T) {
	router, cookie, _, _ := setupVoiceAPI(t)
	_, interviewID := createInterviewFixture(t, router, cookie, nil)
	questionID := firstQuestionID(t, router, cookie, interviewID)
	body := &bytes.Buffer{}
	_, _ = body.WriteString("not a wav file at all, definitely not riff")
	request := httptest.NewRequest(http.MethodPost,
		"/api/interviews/"+jsonNumber(interviewID)+"/questions/"+jsonNumber(questionID)+"/transcribe", body)
	request.Header.Set("Content-Type", "application/octet-stream")
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

// TestVoiceTranscribeRequiresFile 缺文件字段 → 400。
func TestVoiceTranscribeRequiresFile(t *testing.T) {
	router, cookie, _, _ := setupVoiceAPI(t)
	_, interviewID := createInterviewFixture(t, router, cookie, nil)
	questionID := firstQuestionID(t, router, cookie, interviewID)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("wrong", "value")
	_ = writer.Close()

	request := httptest.NewRequest(http.MethodPost,
		"/api/interviews/"+jsonNumber(interviewID)+"/questions/"+jsonNumber(questionID)+"/transcribe", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

// TestVoiceTranscribeReturnsText 上传合法 WAV → 200 {"text":"..."}。
func TestVoiceTranscribeReturnsText(t *testing.T) {
	router, cookie, _, _ := setupVoiceAPI(t)
	_, interviewID := createInterviewFixture(t, router, cookie, nil)
	questionID := firstQuestionID(t, router, cookie, interviewID)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "recording.wav")
	_, _ = part.Write(wavBytes(16000))
	_ = writer.Close()

	path := "/api/interviews/" + strconv.FormatInt(interviewID, 10) + "/questions/" + strconv.FormatInt(questionID, 10) + "/transcribe"
	request := httptest.NewRequest(http.MethodPost, path, body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Text != "识别出来的文字" {
		t.Errorf("text = %q", payload.Text)
	}
}

// TestVoiceTranscribeRejectsUnknownInterview 面试不存在 → 404（不处理上传体）。
func TestVoiceTranscribeRejectsUnknownInterview(t *testing.T) {
	router, cookie, _, _ := setupVoiceAPI(t)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "recording.wav")
	_, _ = part.Write(wavBytes(16000))
	_ = writer.Close()

	request := httptest.NewRequest(http.MethodPost, "/api/interviews/999/questions/1/transcribe", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

// TestQuestionsIncludeAudioState 题目列表每题都带 audioState 字段（值可为空）。
func TestQuestionsIncludeAudioState(t *testing.T) {
	router, cookie, _, _ := setupVoiceAPI(t)
	_, interviewID := createInterviewFixture(t, router, cookie, nil)

	response := requestJSON(t, router, http.MethodGet, "/api/interviews/"+jsonNumber(interviewID)+"/questions", "", cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("questions: %d %s", response.Code, response.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode questions: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("no questions")
	}
	for _, row := range rows {
		if _, ok := row["audioState"]; !ok {
			t.Errorf("question %v missing audioState", row["id"])
		}
	}
}

// TestInterviewDTOIncludesVoiceFlag 面试 DTO 带 voice.enabled（启用 true / 未启用 false）。
func TestInterviewDTOIncludesVoiceFlag(t *testing.T) {
	router, cookie, _, _ := setupVoiceAPI(t)
	_, interviewID := createInterviewFixture(t, router, cookie, nil)
	if enabled := interviewVoiceEnabled(t, router, cookie, interviewID); !enabled {
		t.Error("voice.enabled = false, want true when voice service is wired")
	}

	disabledRouter, disabledCookie, _ := setupInterviewAPI(t)
	_, disabledInterviewID := createInterviewFixture(t, disabledRouter, disabledCookie, nil)
	if enabled := interviewVoiceEnabled(t, disabledRouter, disabledCookie, disabledInterviewID); enabled {
		t.Error("voice.enabled = true, want false when voice service is absent")
	}
}

// interviewVoiceEnabled 读面试详情里的 voice.enabled。
func interviewVoiceEnabled(t *testing.T, router http.Handler, cookie *http.Cookie, interviewID int64) bool {
	t.Helper()
	response := requestJSON(t, router, http.MethodGet, "/api/interviews/"+jsonNumber(interviewID), "", cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("get interview: %d %s", response.Code, response.Body.String())
	}
	var payload struct {
		Voice struct {
			Enabled bool `json:"enabled"`
		} `json:"voice"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode interview: %v", err)
	}
	return payload.Voice.Enabled
}

// interviewTenantID 读回面试所属租户（测试用户的租户由注册时分配，不能写死 1）。
func interviewTenantID(t *testing.T, db *sqlx.DB, interviewID int64) int64 {
	t.Helper()
	var tenantID int64
	if err := db.Get(&tenantID, "SELECT tenant_id FROM interviews WHERE id = ?", interviewID); err != nil {
		t.Fatalf("read interview tenant: %v", err)
	}
	return tenantID
}

// markAudioState 直接把题目的语音状态置为指定值（模拟后台生成器写回）。
func markAudioState(t *testing.T, db *sqlx.DB, questionID int64, state string) {
	t.Helper()
	if _, err := db.Exec("UPDATE interview_questions SET audio_state = ? WHERE id = ?", state, questionID); err != nil {
		t.Fatalf("mark audio state: %v", err)
	}
}
