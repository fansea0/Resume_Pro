package targetjob_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"resumepro/backend/internal/targetjob"
)

func newTestService(t *testing.T) *targetjob.Service {
	t.Helper()
	return targetjob.NewService(targetjob.NewRepository(newTestDB(t)))
}

func TestService_Create_Validation(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, 1, targetjob.CreateInput{Company: "x", Position: "y"})
	require.Error(t, err)
	assert.ErrorIs(t, err, targetjob.ErrValidation)

	_, err = svc.Create(ctx, 1, targetjob.CreateInput{Requirements: "short"})
	require.ErrorIs(t, err, targetjob.ErrValidation)

	tj, err := svc.Create(ctx, 1, targetjob.CreateInput{
		Company:      "字节",
		Requirements: "负责 ToB 营销中台后端架构设计",
	})
	require.NoError(t, err)
	assert.Equal(t, "字节", tj.Company)
	assert.Empty(t, tj.Position)
}

func TestService_Create_RequirementsTrimmed(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, 1, targetjob.CreateInput{Requirements: "          "})
	require.ErrorIs(t, err, targetjob.ErrValidation)
}

func TestService_Create_TrimsRequirementsBeforePersisting(t *testing.T) {
	svc := newTestService(t)

	tj, err := svc.Create(context.Background(), 1, targetjob.CreateInput{Requirements: "  valid requirement  "})
	require.NoError(t, err)
	assert.Equal(t, "valid requirement", tj.Requirements)
}

func TestService_Create_EnforcesFieldLengthLimits(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, 1, targetjob.CreateInput{
		Company:      strings.Repeat("c", 201),
		Requirements: "valid requirement",
	})
	require.ErrorIs(t, err, targetjob.ErrValidation)

	_, err = svc.Create(ctx, 1, targetjob.CreateInput{
		Position:     strings.Repeat("p", 201),
		Requirements: "valid requirement",
	})
	require.ErrorIs(t, err, targetjob.ErrValidation)

	_, err = svc.Create(ctx, 1, targetjob.CreateInput{Requirements: strings.Repeat("r", 20001)})
	require.ErrorIs(t, err, targetjob.ErrValidation)
}

func TestService_Create_AllowsMaxRequirementsWithOptionalFields(t *testing.T) {
	svc := newTestService(t)

	tj, err := svc.Create(context.Background(), 1, targetjob.CreateInput{
		Company:      "Company",
		Position:     "Engineer",
		Requirements: strings.Repeat("r", 20000),
	})
	require.NoError(t, err)
	assert.Len(t, tj.Requirements, 20000)
}

func TestService_Delete_ReturnsReferenceCount(t *testing.T) {
	db := newTestDB(t)
	svc := targetjob.NewService(targetjob.NewRepository(db))
	ctx := context.Background()

	tj, err := svc.Create(ctx, 1, targetjob.CreateInput{Requirements: "test requirement"})
	require.NoError(t, err)
	createResumeReference(t, db, 1, tj.ID)

	deletedCount, err := svc.Delete(ctx, 1, tj.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deletedCount)
	var content string
	require.NoError(t, db.Get(&content, `SELECT content FROM resumes WHERE tenant_id = ?`, 1))
	assert.JSONEq(t, `{}`, content)
	_, err = svc.Get(ctx, 1, tj.ID)
	assert.ErrorIs(t, err, targetjob.ErrNotFound)
}

func TestService_Delete_ReturnsZeroWithoutReferences(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	tj, err := svc.Create(ctx, 1, targetjob.CreateInput{Requirements: "test requirement"})
	require.NoError(t, err)

	deletedCount, err := svc.Delete(ctx, 1, tj.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), deletedCount)
}

func TestService_Update_ValidatesRequirements(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	tj, err := svc.Create(ctx, 1, targetjob.CreateInput{Requirements: "valid requirement here"})
	require.NoError(t, err)

	_, err = svc.Update(ctx, 1, tj.ID, targetjob.UpdateInput{Requirements: "x"})
	require.ErrorIs(t, err, targetjob.ErrValidation)
}
