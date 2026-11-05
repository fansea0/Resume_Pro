package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"resumepro/backend/internal/auth"
	"resumepro/backend/internal/interview"
	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/resumesvc"
	"resumepro/backend/internal/speech"
	"resumepro/backend/internal/store"
	"resumepro/backend/internal/targetjob"
	"resumepro/backend/internal/templates"
)

// stubInterviewModel 是 interview.ResponsesModel 的测试替身：固定返回 payload，
// 生题（QuestionGenerator）与评分（面试图）分别注入不同 payload 的实例。
type stubInterviewModel struct{ payload string }

func (s *stubInterviewModel) Generate(_ context.Context, _ []*schema.AgenticMessage, _ ...model.Option) (*schema.AgenticMessage, error) {
	return &schema.AgenticMessage{
		Role:          schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.AssistantGenText{Text: s.payload})},
	}, nil
}

const stubQuestionPayload = `{"questions":[` +
	`{"question":"Go 的 channel 底层结构？","standardAnswer":"hchan 结构","label":0,"mustPoints":["hchan"],"bonusPoints":["sendq/recvq"]},` +
	`{"question":"项目里的缓存怎么做？","standardAnswer":"","label":2,"mustPoints":["场景"],"bonusPoints":["布隆过滤器"]}` +
	`]}`

const stubGradePayload = `{"grade":"B+","followUp":""}`

// setupInterviewAPI 组装面试路由的测试环境。
// voices 可选：传入语音服务时路由带上语音能力（VoiceService 非 nil）。
func setupInterviewAPI(t *testing.T, voices ...*speech.VoiceService) (http.Handler, *http.Cookie, *sqlx.DB) {
	t.Helper()
	var voice *speech.VoiceService
	if len(voices) > 0 {
		voice = voices[0]
	}
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	authRepo := auth.NewRepository(db)
	resumeRepo := resume.NewRepository(db)
	tmplRepo := templates.NewRepository(db)
	targetSvc := targetjob.NewService(targetjob.NewRepository(db))
	interviewRepo := interview.NewRepository(db)

	mr := miniredis.RunT(t)
	redisStore, err := interview.NewRedisStore(mr.Addr(), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = redisStore.Close() })

	gen := interview.NewQuestionGenerator(&stubInterviewModel{payload: stubQuestionPayload}, 1)
	graph, err := interview.BuildGraph(interviewRepo, &stubInterviewModel{payload: stubGradePayload}, redisStore)
	if err != nil {
		t.Fatal(err)
	}
	interviewSvc := interview.NewService(interviewRepo, redisStore, gen, graph)

	router := NewRouter(authRepo, resumeRepo, tmplRepo, RouterConfig{
		JWTSecret:        "interview-handler-test-secret",
		TargetJobService: targetSvc,
		ResumesvcService: resumesvc.NewService(resumeRepo, nil, targetSvc),
		InterviewService: interviewSvc,
		VoiceService:     voice,
	})
	return router, registerAuthUser(t, router, "interview@example.com"), db
}

// createInterviewFixture 走真实 HTTP 链路：建目标岗位 → 建简历 → 建面试。
func createInterviewFixture(t *testing.T, router http.Handler, cookie *http.Cookie, db *sqlx.DB) (int64, int64) {
	t.Helper()

	targetResponse := requestJSON(t, router, http.MethodPost, "/api/target-jobs",
		`{"company":"字节","position":"后端工程师","requirements":"负责 ToB 营销中台后端架构设计，要求掌握 Go、Redis、MySQL，三年以上经验。"}`, cookie)
	if targetResponse.Code != http.StatusCreated {
		t.Fatalf("create target job: %d %s", targetResponse.Code, targetResponse.Body.String())
	}
	var target struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(targetResponse.Body.Bytes(), &target); err != nil || target.ID == 0 {
		t.Fatalf("decode target job: %v", err)
	}

	resumeResponse := requestJSON(t, router, http.MethodPost, "/api/resumes", validResumeRequest("Backend Engineer", "minimal"), cookie)
	if resumeResponse.Code != http.StatusCreated {
		t.Fatalf("create resume: %d %s", resumeResponse.Code, resumeResponse.Body.String())
	}
	resumeID := decodeResume(t, resumeResponse).ID

	createResponse := requestJSON(t, router, http.MethodPost, "/api/interviews",
		`{"targetJobId":`+jsonNumber(target.ID)+`,"resumeId":`+jsonNumber(resumeID)+`,"config":{"questionCount":2,"labelCounts":{"0":1,"1":0,"2":1},"followUpBudget":1,"perQuestionFollowUpLimit":1}}`, cookie)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create interview: %d %s", createResponse.Code, createResponse.Body.String())
	}
	var created struct {
		Interview struct {
			ID             int64  `json:"id"`
			Status         string `json:"status"`
			ConfigSnapshot struct {
				QuestionCount int `json:"questionCount"`
			} `json:"configSnapshot"`
		} `json:"interview"`
		FirstQuestion map[string]any `json:"firstQuestion"`
	}
	if err := json.Unmarshal(createResponse.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode interview: %v", err)
	}
	if created.Interview.Status != "active" {
		t.Errorf("status = %s, want active", created.Interview.Status)
	}
	if created.Interview.ConfigSnapshot.QuestionCount != 2 {
		t.Errorf("configSnapshot.questionCount = %d, want 2", created.Interview.ConfigSnapshot.QuestionCount)
	}
	if created.FirstQuestion == nil || created.FirstQuestion["question"] == nil {
		t.Error("firstQuestion missing")
	}
	// 面试进行中：首题响应不得携带标准答案与考察点。
	if _, leaked := created.FirstQuestion["standardAnswer"]; leaked {
		t.Error("firstQuestion leaked standardAnswer while interview is active")
	}
	if _, leaked := created.FirstQuestion["extra"]; leaked {
		t.Error("firstQuestion leaked extra while interview is active")
	}
	return target.ID, created.Interview.ID
}

func TestInterviewHandlerCreateAndAnswer(t *testing.T) {
	router, cookie, db := setupInterviewAPI(t)
	_, interviewID := createInterviewFixture(t, router, cookie, db)

	answerResponse := requestJSON(t, router, http.MethodPost, "/api/interviews/"+jsonNumber(interviewID)+"/answer",
		`{"answer":"channel 是 hchan 结构，包含 sendq 和 recvq 环形缓冲区"}`, cookie)
	if answerResponse.Code != http.StatusOK {
		t.Fatalf("answer: %d %s", answerResponse.Code, answerResponse.Body.String())
	}
	var result struct {
		Grade        string         `json:"grade"`
		FollowUp     string         `json:"followUp"`
		Finished     bool           `json:"finished"`
		NextQuestion map[string]any `json:"nextQuestion"`
	}
	if err := json.Unmarshal(answerResponse.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode answer: %v", err)
	}
	if result.Grade != "B+" {
		t.Errorf("grade = %s, want B+", result.Grade)
	}
	if result.Finished {
		t.Error("finished = true, want false mid-way")
	}
	if result.NextQuestion == nil {
		t.Error("nextQuestion missing after first answer")
	}
	if _, leaked := result.NextQuestion["standardAnswer"]; leaked {
		t.Error("nextQuestion leaked standardAnswer while interview is active")
	}
	if _, leaked := result.NextQuestion["extra"]; leaked {
		t.Error("nextQuestion leaked extra while interview is active")
	}

	questionsResponse := requestJSON(t, router, http.MethodGet, "/api/interviews/"+jsonNumber(interviewID)+"/questions", "", cookie)
	if questionsResponse.Code != http.StatusOK {
		t.Fatalf("questions: %d %s", questionsResponse.Code, questionsResponse.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(questionsResponse.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode questions: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("questions = %d, want 2", len(rows))
	}
	if rows[0]["label"] != "0" {
		t.Errorf("label = %v, want string \"0\"", rows[0]["label"])
	}
	if rows[0]["userAnswer"] == nil {
		t.Error("userAnswer should be written after answer")
	}
	for _, row := range rows {
		if _, leaked := row["standardAnswer"]; leaked {
			t.Errorf("questions leaked standardAnswer while interview is active: %v", row["id"])
		}
		if _, leaked := row["extra"]; leaked {
			t.Errorf("questions leaked extra while interview is active: %v", row["id"])
		}
	}
}

// TestInterviewHandlerSolutionsAfterCompletion 验证面试结束后标准答案与考察点恢复下发，
// 同时面试进行中（active）不得出现。
func TestInterviewHandlerSolutionsAfterCompletion(t *testing.T) {
	router, cookie, db := setupInterviewAPI(t)
	_, interviewID := createInterviewFixture(t, router, cookie, db)

	path := "/api/interviews/" + jsonNumber(interviewID) + "/answer"
	first := requestJSON(t, router, http.MethodPost, path, `{"answer":"hchan 结构"}`, cookie)
	if first.Code != http.StatusOK {
		t.Fatalf("first answer: %d %s", first.Code, first.Body.String())
	}
	second := requestJSON(t, router, http.MethodPost, path, `{"answer":"按业务场景选择缓存方案"}`, cookie)
	if second.Code != http.StatusOK {
		t.Fatalf("second answer: %d %s", second.Code, second.Body.String())
	}
	var result struct {
		Finished bool `json:"finished"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode answer: %v", err)
	}
	if !result.Finished {
		t.Fatal("finished = false, want true after the last question")
	}

	questionsResponse := requestJSON(t, router, http.MethodGet, "/api/interviews/"+jsonNumber(interviewID)+"/questions", "", cookie)
	if questionsResponse.Code != http.StatusOK {
		t.Fatalf("questions: %d %s", questionsResponse.Code, questionsResponse.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(questionsResponse.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode questions: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("no questions after completion")
	}
	if rows[0]["standardAnswer"] != "hchan 结构" {
		t.Errorf("standardAnswer = %v, want recovered after completion", rows[0]["standardAnswer"])
	}
	if rows[0]["extra"] == nil {
		t.Error("extra should be recovered after completion")
	}
}

func TestInterviewHandlerAnswerNotFound(t *testing.T) {
	router, cookie, _ := setupInterviewAPI(t)
	response := requestJSON(t, router, http.MethodPost, "/api/interviews/999/answer",
		`{"answer":"x"}`, cookie)
	if response.Code != http.StatusNotFound {
		t.Fatalf("answer unknown interview: %d, want 404", response.Code)
	}
}

func TestInterviewHandlerCreateConflict(t *testing.T) {
	router, cookie, _ := setupInterviewAPI(t)
	_, _ = createInterviewFixture(t, router, cookie, nil)

	// 第二次创建同一租户的面试 → 409（ErrActiveInterviewExists）
	targetResponse := requestJSON(t, router, http.MethodPost, "/api/target-jobs",
		`{"company":"另一家","position":"后端","requirements":"第二个目标岗位，用于复测冲突，要求内容足够长满足校验。"}`, cookie)
	var secondTarget struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(targetResponse.Body.Bytes(), &secondTarget); err != nil || secondTarget.ID == 0 {
		t.Fatalf("create second target job: %v", err)
	}
	resumeResponse := requestJSON(t, router, http.MethodPost, "/api/resumes", validResumeRequest("Second", "minimal"), cookie)
	secondResumeID := decodeResume(t, resumeResponse).ID

	conflict := requestJSON(t, router, http.MethodPost, "/api/interviews",
		`{"targetJobId":`+jsonNumber(secondTarget.ID)+`,"resumeId":`+jsonNumber(secondResumeID)+`,"config":{"questionCount":2,"labelCounts":{"0":1,"1":0,"2":1},"followUpBudget":1,"perQuestionFollowUpLimit":1}}`, cookie)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("second create = %d, want 409: %s", conflict.Code, conflict.Body.String())
	}
}
