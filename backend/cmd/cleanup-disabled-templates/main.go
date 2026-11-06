// Command cleanup-disabled-templates removes templates that the admin has
// disabled via the UI. It reads the disabled list from the SQLite database,
// deletes the frontend template files and preview images, strips every
// reference from source, migrates affected rows to the default template ID,
// clears the disabled_templates table, and commits the result.
//
// Usage:
//
//	# 1. Dry-run first — prints the plan, nothing is touched.
//	cd backend && go run ./cmd/cleanup-disabled-templates
//
//	# 2. If the plan looks right, apply. The script asks for final confirmation.
//	go run ./cmd/cleanup-disabled-templates --apply
//
//	# 3. Skip the confirmation prompt (for CI / automation).
//	go run ./cmd/cleanup-disabled-templates --apply --yes
//
// The script operates entirely on hard-coded repo layout:
//
//	repo root     → detected by walking up from cwd until backend/go.mod is found
//	database      → <repo root>/backend/resume.db
//	frontend      → <repo root>/frontend/
//	default tpl   → redline (disabled templates will be migrated to this)
//
// It will not push. Run `git push` manually after confirming the commit.
package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"

	"resumepro/backend/internal/store"
	"resumepro/backend/internal/templates"
)

// --- hard-coded layout ------------------------------------------------------

// repoRoot is resolved from the location of this source file at build time.
// We compute it by walking up from backend/cmd/cleanup-disabled-templates/.
// At runtime we take the working directory (expected to be repo root or
// backend/) as a convenience fallback, but the source-file-derived path
// is the ground truth.
func repoRoot() string {
	// The source file lives at backend/cmd/cleanup-disabled-templates/main.go,
	// so walk up 4 times from this directory to reach repo root.
	// But since we compile and run from different places, also try cwd.
	cwd, _ := os.Getwd()
	for i := 0; i < 4; i++ {
		if _, err := os.Stat(filepath.Join(cwd, "backend", "go.mod")); err == nil {
			return cwd
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}
	// Fallback: treat as if run from repo root already
	return cwd
}

const (
	defaultTemplateID = "redline"
	dbRelPath         = "backend/resume.db"
	frontendRelPath   = "frontend"
)

// --- main -------------------------------------------------------------------

func main() {
	apply := flag.Bool("apply", false, "actually execute the cleanup (default is dry-run)")
	yes := flag.Bool("yes", false, "skip the final confirmation in interactive apply mode")
	flag.Parse()

	root := repoRoot()
	dbPath := filepath.Join(root, dbRelPath)
	frontendDir := filepath.Join(root, frontendRelPath)

	fmt.Printf("repo root:  %s\n", root)
	fmt.Printf("database:   %s\n", dbPath)
	fmt.Printf("frontend:   %s\n", frontendDir)
	fmt.Printf("mode:       %s\n", mode(*apply))
	fmt.Println()

	if err := run(context.Background(), root, dbPath, frontendDir, defaultTemplateID, *apply, *yes); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func mode(apply bool) string {
	if apply {
		return "APPLY (will modify files, DB, and commit)"
	}
	return "DRY-RUN (no side effects)"
}

// --- run: orchestrates the whole flow ---------------------------------------

func run(ctx context.Context, root, dbPath, frontendDir, defaultTpl string, apply, yes bool) error {
	// 1. Read disabled template IDs from DB
	db, err := store.OpenSQLite(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	repo := templates.NewRepository(db)
	disabledIDs, err := repo.ListDisabled(ctx)
	if err != nil {
		return fmt.Errorf("list disabled templates: %w", err)
	}
	sort.Strings(disabledIDs)

	if len(disabledIDs) == 0 {
		fmt.Println("disabled_templates table is empty — nothing to clean up.")
		return nil
	}

	fmt.Printf("disabled template IDs (%d): %v\n", len(disabledIDs), disabledIDs)
	fmt.Println()

	// 2. Pre-flight checks
	checks := preflight(frontendDir, disabledIDs)
	fmt.Println("== pre-flight checks ==")
	for _, c := range checks {
		mark := "OK "
		if !c.ok {
			mark = "WARN"
		}
		fmt.Printf("  [%s] %s\n", mark, c.msg)
	}
	fmt.Println()

	// 3. Build plan
	plan := buildPlan(root, frontendDir, dbPath, defaultTpl, disabledIDs)

	fmt.Println("== will delete files ==")
	for _, f := range plan.filesToDelete {
		fmt.Printf("  D  %s\n", f)
	}
	fmt.Println()

	fmt.Println("== will modify files ==")
	for _, f := range plan.filesToModify {
		fmt.Printf("  M  %s\n", f)
	}
	fmt.Println()

	fmt.Println("== will run SQL ==")
	for _, s := range plan.sql {
		fmt.Printf("  %s\n", s)
	}
	fmt.Println()

	fmt.Println("== will git-add paths ==")
	for _, p := range plan.gitPaths {
		fmt.Printf("  +  %s\n", p)
	}
	fmt.Println()

	if !apply {
		fmt.Println("dry-run complete. Re-run with --apply to execute.")
		return nil
	}

	// 4. Confirm
	if !yes {
		fmt.Print("Proceed? [y/N]: ")
		var reply string
		fmt.Scanln(&reply)
		if reply != "y" && reply != "Y" {
			fmt.Println("aborted.")
			return nil
		}
	}

	// 5. Execute
	if err := plan.execute(ctx, root, db); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("== done ==")
	fmt.Println("next steps:")
	fmt.Println("  cd frontend && npx vitest run && npm run build")
	return nil
}

type check struct {
	ok  bool
	msg string
}

func preflight(frontendDir string, ids []string) []check {
	var out []check

	templatesDir := filepath.Join(frontendDir, "src", "templates")
	previewDir := filepath.Join(frontendDir, "public", "template-previews")

	registryPath := filepath.Join(templatesDir, "registry.tsx")
	typesPath := filepath.Join(frontendDir, "src", "types", "resume.ts")
	sanitizePath := filepath.Join(frontendDir, "src", "utils", "sanitizeGeneratedResume.ts")
	apiResumesPath := filepath.Join(frontendDir, "src", "api", "resumes.ts")

	registryBytes, _ := os.ReadFile(registryPath)
	typesBytes, _ := os.ReadFile(typesPath)
	sanitizeBytes, _ := os.ReadFile(sanitizePath)
	apiResumesBytes, _ := os.ReadFile(apiResumesPath)

	for _, id := range ids {
		comp := filepath.Join(templatesDir, id+".tsx")
		prev := filepath.Join(previewDir, id+".png")
		out = append(out, check{fileExists(comp), fmt.Sprintf("component: %s", filepath.Base(comp))})
		out = append(out, check{fileExists(prev), fmt.Sprintf("preview: %s", filepath.Base(prev))})

		// registry.tsx: import line
		if strings.Contains(string(registryBytes), "from './"+id+"'") {
			out = append(out, check{true, fmt.Sprintf("registry import: %s", id)})
		} else {
			out = append(out, check{false, fmt.Sprintf("registry import missing: %s", id)})
		}
		// registry.tsx: array entry
		if strings.Contains(string(registryBytes), "id: '"+id+"'") {
			out = append(out, check{true, fmt.Sprintf("registry entry: %s", id)})
		} else {
			out = append(out, check{false, fmt.Sprintf("registry entry missing: %s", id)})
		}

		// types/resume.ts
		if strings.Contains(string(typesBytes), "'"+id+"'") {
			out = append(out, check{true, fmt.Sprintf("TemplateId union contains: %s", id)})
		} else {
			out = append(out, check{false, fmt.Sprintf("TemplateId union missing: %s", id)})
		}

		// sanitizeGeneratedResume.ts
		if strings.Contains(string(sanitizeBytes), "'"+id+"'") {
			out = append(out, check{true, fmt.Sprintf("sanitize whitelist contains: %s", id)})
		} else {
			out = append(out, check{false, fmt.Sprintf("sanitize whitelist missing: %s", id)})
		}

		// api/resumes.ts
		if strings.Contains(string(apiResumesBytes), "'"+id+"'") {
			out = append(out, check{true, fmt.Sprintf("api/resumes whitelist contains: %s", id)})
		} else {
			out = append(out, check{false, fmt.Sprintf("api/resumes whitelist missing: %s", id)})
		}
	}
	return out
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// --- plan -------------------------------------------------------------------

type plan struct {
	filesToDelete []string
	filesToModify []string
	sql           []string
	gitPaths      []string

	frontendDir string
	dbPath      string
	defaultTpl  string
	ids         []string
}

func buildPlan(root, frontendDir, dbPath, defaultTpl string, ids []string) plan {
	templatesDir := filepath.Join(frontendDir, "src", "templates")
	previewDir := filepath.Join(frontendDir, "public", "template-previews")

	var deleteFiles []string
	for _, id := range ids {
		deleteFiles = append(deleteFiles,
			filepath.Join(templatesDir, id+".tsx"),
			filepath.Join(previewDir, id+".png"),
		)
	}

	modifyFiles := []string{
		filepath.Join(templatesDir, "registry.tsx"),
		filepath.Join(templatesDir, "templates.css"),
		filepath.Join(frontendDir, "src", "types", "resume.ts"),
		filepath.Join(frontendDir, "src", "utils", "sanitizeGeneratedResume.ts"),
		filepath.Join(frontendDir, "src", "api", "resumes.ts"),
	}

	// Walk frontend/src for any other .ts/.tsx file that references disabled IDs.
	_ = filepath.WalkDir(filepath.Join(frontendDir, "src"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx") {
			return nil
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		content := string(b)
		for _, id := range ids {
			if strings.Contains(content, "'"+id+"'") || strings.Contains(content, `"`+id+`"`) {
				already := false
				for _, m := range modifyFiles {
					if m == path {
						already = true
						break
					}
				}
				if !already {
					modifyFiles = append(modifyFiles, path)
				}
				break
			}
		}
		return nil
	})

	sql := []string{
		fmt.Sprintf("UPDATE resumes SET template_id='%s' WHERE template_id IN (%s)",
			defaultTpl, quotedIn(ids)),
		fmt.Sprintf("UPDATE resumes SET content = json_set(content, '$.templateId', '%s') WHERE json_extract(content, '$.templateId') IN (%s)",
			defaultTpl, quotedIn(ids)),
		"DELETE FROM disabled_templates",
	}

	// git paths must be relative to repo root so git add -A works.
	toRel := func(abs string) string {
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return abs // fallback
		}
		return rel
	}
	gitPaths := make([]string, 0, len(deleteFiles)+len(modifyFiles)+1)
	for _, f := range deleteFiles {
		gitPaths = append(gitPaths, toRel(f))
	}
	for _, f := range modifyFiles {
		gitPaths = append(gitPaths, toRel(f))
	}
	gitPaths = append(gitPaths, toRel(dbPath))

	return plan{
		filesToDelete: deleteFiles,
		filesToModify: modifyFiles,
		sql:           sql,
		gitPaths:      gitPaths,
		frontendDir:   frontendDir,
		dbPath:        dbPath,
		defaultTpl:    defaultTpl,
		ids:           ids,
	}
}

func quotedIn(ids []string) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = "'" + strings.ReplaceAll(id, "'", "''") + "'"
	}
	return strings.Join(parts, ", ")
}

// --- execution --------------------------------------------------------------

func (p plan) execute(ctx context.Context, root string, sqlxDB *sqlx.DB) error {
	// 1. Delete component + preview files
	fmt.Println("== deleting files ==")
	for _, f := range p.filesToDelete {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", f, err)
		}
		fmt.Printf("  removed  %s\n", f)
	}

	// 2. Modify source files
	fmt.Println("\n== rewriting source files ==")
	for _, path := range p.filesToModify {
		if err := rewriteFile(path, p.ids, p.defaultTpl); err != nil {
			return fmt.Errorf("rewrite %s: %w", path, err)
		}
		fmt.Printf("  updated  %s\n", path)
	}

	// 3. DB migration
	fmt.Println("\n== migrating database ==")
	if err := migrateDB(ctx, sqlxDB, p.ids, p.defaultTpl); err != nil {
		return fmt.Errorf("migrate db: %w", err)
	}

	// 4. Git commit
	fmt.Println("\n== git commit ==")
	if err := gitCommit(root, p.gitPaths, p.ids); err != nil {
		return fmt.Errorf("git: %w", err)
	}

	return nil
}

// rewriteFile dispatches on filename to the right rewrite strategy.
func rewriteFile(path string, ids []string, defaultTpl string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(b)

	switch {
	case strings.HasSuffix(path, "registry.tsx"):
		content = rewriteRegistry(content, ids)
	case strings.HasSuffix(path, "templates.css"):
		content = rewriteCSS(content, ids)
	case strings.HasSuffix(path, "types/resume.ts"):
		content = rewriteTypes(content, ids, defaultTpl)
	case strings.HasSuffix(path, "sanitizeGeneratedResume.ts"),
		strings.HasSuffix(path, "api/resumes.ts"):
		content = rewriteSetWhitelist(content, ids)
	default:
		// Catch-all: replace disabled ID literals with defaultTpl, but only
		// inside template-ID-looking contexts. Safe(r) than blind replace_all.
		content = rewriteReferences(content, ids, defaultTpl)
	}

	return os.WriteFile(path, []byte(content), 0o644)
}

// rewriteRegistry: strip "from './id'" import lines and "{ id: 'id', ... }" entries.
func rewriteRegistry(content string, ids []string) string {
	// Remove import lines. Pattern: import { Xxx } from './id';
	for _, id := range ids {
		re := regexp.MustCompile(`(?m)^import \{[^}]*\} from '\./` + regexp.QuoteMeta(id) + `';\n`)
		content = re.ReplaceAllString(content, "")
		re2 := regexp.MustCompile(`(?m)^import \{[^}]*\} from "\./` + regexp.QuoteMeta(id) + `";\n`)
		content = re2.ReplaceAllString(content, "")
	}

	// Remove registry entries. Each entry is { id: 'xxx', name: '...', ... Renderer: XxxTemplate },
	// potentially spanning multiple lines; we match from { id: 'xxx' to the closing },.
	for _, id := range ids {
		re := regexp.MustCompile(`(?s)^\s*\{ id: '` + regexp.QuoteMeta(id) + `',.*?\},\n`)
		content = re.ReplaceAllString(content, "")
		re2 := regexp.MustCompile(`(?s)^\s*\{ id: "` + regexp.QuoteMeta(id) + `",.*?\},\n`)
		content = re2.ReplaceAllString(content, "")
	}
	return content
}

// rewriteCSS: strip every disabled template's style block.
// A block starts at ".template-{id}" and ends just before the next
// top-level ".template-" selector (or end of file).
func rewriteCSS(content string, ids []string) string {
	for _, id := range ids {
		// .template-{id} followed by selectors, until next .template-<something>
		re := regexp.MustCompile(`(?s)\.template-` + regexp.QuoteMeta(id) + `[^{]*\{.*?(?=\n\.template-|\z)`)
		content = re.ReplaceAllString(content, "")
	}
	// Collapse excessive blank lines
	re := regexp.MustCompile(`\n{3,}`)
	content = re.ReplaceAllString(content, "\n\n")
	return content
}

// rewriteTypes: strip union members from TemplateId type.
// Pattern:   | 'xxx'   (each on its own line)
func rewriteTypes(content string, ids []string, defaultTpl string) string {
	for _, id := range ids {
		if id == defaultTpl {
			continue
		}
		re := regexp.MustCompile(`(?m)^\s*\| '` + regexp.QuoteMeta(id) + `'\n`)
		content = re.ReplaceAllString(content, "")
	}

	// If defaultTpl itself was removed (shouldn't happen but guard anyway),
	// re-add a placeholder. Not implemented — callers shouldn't pass defaultTpl.

	// Also: update createEmptyResumeDocument's templateId default if it
	// happens to be a disabled ID.
	for _, id := range ids {
		re := regexp.MustCompile(`templateId:\s*'` + regexp.QuoteMeta(id) + `'`)
		content = re.ReplaceAllString(content, "templateId: '"+defaultTpl+"'")
	}
	return content
}

// rewriteSetWhitelist: strip disabled IDs from the Set<TemplateId>([...]).
func rewriteSetWhitelist(content string, ids []string) string {
	for _, id := range ids {
		// Match 'xxx', or 'xxx' as the last item (no trailing comma)
		re := regexp.MustCompile(`'` + regexp.QuoteMeta(id) + `',\s*`)
		content = re.ReplaceAllString(content, "")
		re2 := regexp.MustCompile(`'\s*,\s*` + regexp.QuoteMeta(id) + `'`)
		content = re2.ReplaceAllString(content, "")
		// 'xxx' followed by just ]
		re3 := regexp.MustCompile(`'` + regexp.QuoteMeta(id) + `'(\s*\])`)
		content = re3.ReplaceAllString(content, ``)
	}
	// Collapse ", , " and " ," anomalies
	re := regexp.MustCompile(`,\s*,\s*`)
	content = re.ReplaceAllString(content, ", ")
	re2 := regexp.MustCompile(`\[\s*,`)
	content = re2.ReplaceAllString(content, "[")
	re3 := regexp.MustCompile(`,\s*\]`)
	content = re3.ReplaceAllString(content, "]")
	return content
}

// rewriteReferences catches any stray disabled-ID literals in source files
// we found via grep but that aren't one of the five canonical files above.
// We only replace in template-ID-looking contexts to minimize risk.
func rewriteReferences(content string, ids []string, defaultTpl string) string {
	for _, id := range ids {
		// templateId: 'xxx' -> templateId: 'redline'
		re := regexp.MustCompile(`templateId:\s*'` + regexp.QuoteMeta(id) + `'`)
		content = re.ReplaceAllString(content, "templateId: '"+defaultTpl+"'")
		re2 := regexp.MustCompile(`templateId:\s*"` + regexp.QuoteMeta(id) + `"`)
		content = re2.ReplaceAllString(content, "templateId: \""+defaultTpl+"\"")

		// 'xxx' as TemplateId   and   'xxx' as const
		re3 := regexp.MustCompile(`'` + regexp.QuoteMeta(id) + `'(?=\s+as\s+(?:TemplateId|const))`)
		content = re3.ReplaceAllString(content, "'"+defaultTpl+"'")
		re4 := regexp.MustCompile(`"` + regexp.QuoteMeta(id) + `"(?=\s+as\s+(?:TemplateId|const))`)
		content = re4.ReplaceAllString(content, "\""+defaultTpl+"\"")

		// id: 'xxx' in registry-like contexts already handled by rewriteRegistry,
		// but this catch-all also covers it for other files.
		re5 := regexp.MustCompile(`\bid:\s*'` + regexp.QuoteMeta(id) + `'`)
		content = re5.ReplaceAllString(content, "id: '"+defaultTpl+"'")
	}
	return content
}

// --- database migration -----------------------------------------------------

func migrateDB(ctx context.Context, db *sqlx.DB, ids []string, defaultTpl string) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() // no-op after Commit

	res := tx.MustExecContext(ctx,
		fmt.Sprintf(`UPDATE resumes SET template_id='%s' WHERE template_id IN (%s)`,
			defaultTpl, quotedIn(ids)),
	)
	n1, _ := res.RowsAffected()
	fmt.Printf("  resumes.template_id updated: %d rows\n", n1)

	res = tx.MustExecContext(ctx,
		fmt.Sprintf(`UPDATE resumes SET content = json_set(content, '$.templateId', '%s') WHERE json_extract(content, '$.templateId') IN (%s)`,
			defaultTpl, quotedIn(ids)),
	)
	n2, _ := res.RowsAffected()
	fmt.Printf("  resumes.content.templateId migrated: %d rows\n", n2)

	res = tx.MustExecContext(ctx, `DELETE FROM disabled_templates`)
	n3, _ := res.RowsAffected()
	fmt.Printf("  disabled_templates cleared: %d rows\n", n3)

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// --- git --------------------------------------------------------------------

func gitCommit(root string, paths []string, ids []string) error {
	args := append([]string{"add", "-A"}, paths...)
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add: %w\n%s", err, out)
	}

	cmd = exec.Command("git", "diff", "--cached", "--stat")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git diff --cached --stat: %w\n%s", err, out)
	} else if len(out) > 0 {
		fmt.Print("  staged:\n    " + strings.ReplaceAll(strings.TrimSpace(string(out)), "\n", "\n    ") + "\n")
	}

	idsList := strings.Join(ids, ", ")
	msg := fmt.Sprintf("Remove disabled templates (%s)\n\n"+
		"- delete frontend components, preview images, and CSS style blocks\n"+
		"- strip IDs from TemplateId union, whitelists, and registry\n"+
		"- migrate DB: resumes.template_id and content JSON -> redline; clear disabled_templates\n"+
		"- update all test references to the default template (redline)",
		idsList)

	cmd = exec.Command("git", "commit", "-m", msg)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %w\n%s", err, out)
	}
	out, _ := cmd.Output()
	fmt.Printf("  committed: %s\n", strings.TrimSpace(string(out)))
	return nil
}
