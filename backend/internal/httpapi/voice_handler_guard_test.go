package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

// insertFollowUpQuestion 插入一道追问（parent_id 指向主问题），返回追问行 id。
func insertFollowUpQuestion(t *testing.T, db *sqlx.DB, interviewID, parentID int64, text string) int64 {
	t.Helper()
	result, err := db.Exec(
		"INSERT INTO interview_questions (interview_id, parent_id, follow_up_round, label, question, standard_answer, user_answer, extra, grade, created_at) "+
			"VALUES (?, ?, 1, 0, ?, NULL, NULL, NULL, NULL, ?)",
		interviewID, parentID, text, time.Now().UTC())
	if err != nil {
		t.Fatalf("insert follow-up question: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("follow-up last insert id: %v", err)
	}
	return id
}

// TestQuestionsIncludeFollowUpAudioState 追问链（QuestionChain 查询）也必须带出 audio_state。
//
// MainQuestions 与 QuestionChain 是两条独立 SQL；只测主问题会漏掉追问那条路径。
func TestQuestionsIncludeFollowUpAudioState(t *testing.T) {
	router, cookie, db, _ := setupVoiceAPI(t)
	_, interviewID := createInterviewFixture(t, router, cookie, nil)
	questionID := firstQuestionID(t, router, cookie, interviewID)
	followUpID := insertFollowUpQuestion(t, db, interviewID, questionID, "追问：channel 为什么要做缓冲")
	markAudioState(t, db, followUpID, "ready")

	response := requestJSON(t, router, http.MethodGet, "/api/interviews/"+jsonNumber(interviewID)+"/questions", "", cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("questions: %d %s", response.Code, response.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode questions: %v", err)
	}
	for _, row := range rows {
		id, ok := row["id"].(float64)
		if !ok || int64(id) != followUpID {
			continue
		}
		if row["audioState"] != "ready" {
			t.Errorf("follow-up audioState = %v, want ready", row["audioState"])
		}
		return
	}
	t.Errorf("follow-up question %d not present in questions list", followUpID)
}

// TestVoiceAudioRejectsCrossTenant 别的租户访问这场面试的音频 → 404。
func TestVoiceAudioRejectsCrossTenant(t *testing.T) {
	router, cookie, _, _ := setupVoiceAPI(t)
	_, interviewID := createInterviewFixture(t, router, cookie, nil)
	questionID := firstQuestionID(t, router, cookie, interviewID)

	otherCookie := registerAuthUser(t, router, "voice-other@example.com")
	response := requestJSON(t, router, http.MethodGet,
		"/api/interviews/"+jsonNumber(interviewID)+"/questions/"+jsonNumber(questionID)+"/audio", "", otherCookie)
	if response.Code != http.StatusNotFound {
		t.Fatalf("cross tenant audio = %d, want 404: %s", response.Code, response.Body.String())
	}
}

// TestVoiceAudioRejectsForeignQuestion 自己的面试 + 别人的题目 id → 404。
func TestVoiceAudioRejectsForeignQuestion(t *testing.T) {
	router, cookie, _, _ := setupVoiceAPI(t)
	_, interviewID := createInterviewFixture(t, router, cookie, nil)
	questionID := firstQuestionID(t, router, cookie, interviewID)

	// 同一套路由（同一个库）里，第二个用户属于另一个租户。
	otherCookie := registerAuthUser(t, router, "voice-foreign@example.com")
	_, otherInterviewID := createInterviewFixture(t, router, otherCookie, nil)
	if otherInterviewID == interviewID {
		t.Fatal("expected a distinct interview id")
	}
	// 自己的面试 + 别人的题目 id → 404。
	response := requestJSON(t, router, http.MethodGet,
		"/api/interviews/"+jsonNumber(otherInterviewID)+"/questions/"+jsonNumber(questionID)+"/audio", "", otherCookie)
	if response.Code != http.StatusNotFound {
		t.Fatalf("foreign question = %d, want 404: %s", response.Code, response.Body.String())
	}
}

// TestVoiceTranscribeRejectsCrossTenant 别的租户转写 → 404（不处理上传体）。
func TestVoiceTranscribeRejectsCrossTenant(t *testing.T) {
	router, cookie, _, _ := setupVoiceAPI(t)
	_, interviewID := createInterviewFixture(t, router, cookie, nil)
	questionID := firstQuestionID(t, router, cookie, interviewID)

	otherCookie := registerAuthUser(t, router, "voice-other-transcribe@example.com")
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "recording.wav")
	_, _ = part.Write(wavBytes(16000))
	_ = writer.Close()

	request := httptest.NewRequest(http.MethodPost,
		"/api/interviews/"+jsonNumber(interviewID)+"/questions/"+jsonNumber(questionID)+"/transcribe", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.AddCookie(otherCookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("cross tenant transcribe = %d, want 404: %s", recorder.Code, recorder.Body.String())
	}
}

// TestVoiceTranscribeRejectsOversizedBody 请求体超限 → 400，且不在磁盘留下 multipart 临时文件。
//
// multipart 解析会把整个请求体读进内存，超出内存阈值的部分写成磁盘临时文件；
// 不在解析前拦住的话，用户录音会短暂落盘，与「不存录音」的约束冲突。
func TestVoiceTranscribeRejectsOversizedBody(t *testing.T) {
	router, cookie, _, _ := setupVoiceAPI(t)
	_, interviewID := createInterviewFixture(t, router, cookie, nil)
	questionID := firstQuestionID(t, router, cookie, interviewID)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "recording.wav")
	_, _ = part.Write(bytes.Repeat([]byte{0}, 25<<20))
	_ = writer.Close()

	before := countMultipartTempFiles(t)
	request := httptest.NewRequest(http.MethodPost,
		"/api/interviews/"+jsonNumber(interviewID)+"/questions/"+jsonNumber(questionID)+"/transcribe", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("oversized upload = %d, want 400: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "too large") {
		t.Errorf("body = %s, want a too-large error", recorder.Body.String())
	}
	if after := countMultipartTempFiles(t); after != before {
		t.Errorf("multipart temp files = %d, want %d (录音不应落盘)", after, before)
	}

	// chunked 请求没有 Content-Length，只能靠 MaxBytesReader 兜底：
	// 少了它，整个请求体会被读进内存并把超出部分写成磁盘临时文件。
	chunked := &bytes.Buffer{}
	chunkedWriter := multipart.NewWriter(chunked)
	chunkedPart, _ := chunkedWriter.CreateFormFile("file", "recording.wav")
	_, _ = chunkedPart.Write(bytes.Repeat([]byte{0}, 25<<20))
	_ = chunkedWriter.Close()

	chunkedRequest := httptest.NewRequest(http.MethodPost,
		"/api/interviews/"+jsonNumber(interviewID)+"/questions/"+jsonNumber(questionID)+"/transcribe", chunked)
	chunkedRequest.Header.Set("Content-Type", chunkedWriter.FormDataContentType())
	chunkedRequest.ContentLength = -1
	chunkedRequest.AddCookie(cookie)
	chunkedRecorder := httptest.NewRecorder()
	router.ServeHTTP(chunkedRecorder, chunkedRequest)
	if chunkedRecorder.Code != http.StatusBadRequest {
		t.Fatalf("oversized chunked upload = %d, want 400: %s", chunkedRecorder.Code, chunkedRecorder.Body.String())
	}
	if !strings.Contains(chunkedRecorder.Body.String(), "too large") {
		t.Errorf("chunked body = %s, want a too-large error", chunkedRecorder.Body.String())
	}
	if after := countMultipartTempFiles(t); after != before {
		t.Errorf("multipart temp files after chunked upload = %d, want %d (录音不应落盘)", after, before)
	}
}

// countMultipartTempFiles 统计临时目录里的 multipart 临时文件数量。
func countMultipartTempFiles(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		t.Fatalf("read temp dir: %v", err)
	}
	count := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "multipart-") && !entry.IsDir() {
			count++
		}
	}
	return count
}
