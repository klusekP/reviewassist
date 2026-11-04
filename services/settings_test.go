package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "sub", "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func viewOf(m *SettingsManager, key string) SettingView {
	_, views := m.View()
	for _, v := range views {
		if v.Key == key {
			return v
		}
	}
	return SettingView{}
}

func TestSettingsPrecedenceAndReset(t *testing.T) {
	t.Setenv("REVIEW_LANGUAGE", "en")
	t.Setenv("GITLAB_TOKEN", "glpat-from-env-1234")
	os.Unsetenv("REVIEW_MAX_FILES")
	store := openTestStore(t)
	m, err := NewSettingsManager(store)
	if err != nil {
		t.Fatal(err)
	}

	// env and default sources
	if v := viewOf(m, "REVIEW_LANGUAGE"); v.Value != "en" || v.Source != "env" {
		t.Errorf("REVIEW_LANGUAGE = %+v", v)
	}
	if v := viewOf(m, "REVIEW_MAX_FILES"); v.Value != "30" || v.Source != "default" {
		t.Errorf("REVIEW_MAX_FILES = %+v", v)
	}
	// secrets are masked
	if v := viewOf(m, "GITLAB_TOKEN"); v.Value != "••••1234" || strings.Contains(v.Value, "glpat") {
		t.Errorf("secret not masked: %q", v.Value)
	}

	// UI override wins and is exported to the environment
	if _, err := m.Update(map[string]string{"REVIEW_LANGUAGE": "pl", "REVIEW_MAX_FILES": "12", "GITLAB_TOKEN": ""}, nil); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("REVIEW_LANGUAGE") != "pl" || viewOf(m, "REVIEW_LANGUAGE").Source != "ui" {
		t.Error("UI value not applied")
	}
	if os.Getenv("GITLAB_TOKEN") != "glpat-from-env-1234" {
		t.Error("empty secret must keep the current value")
	}

	// persisted: a new manager (app restart) applies UI values over .env;
	// simulate a fresh process environment (.env has only REVIEW_LANGUAGE=en)
	t.Setenv("REVIEW_LANGUAGE", "en")
	os.Unsetenv("REVIEW_MAX_FILES")
	m2, err := NewSettingsManager(store)
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("REVIEW_LANGUAGE") != "pl" || viewOf(m2, "REVIEW_MAX_FILES").Value != "12" {
		t.Error("UI settings not restored after restart")
	}

	// reset falls back to .env, or unsets when there is no .env value
	if _, err := m2.Update(nil, []string{"REVIEW_LANGUAGE", "REVIEW_MAX_FILES"}); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("REVIEW_LANGUAGE") != "en" || viewOf(m2, "REVIEW_LANGUAGE").Source != "env" {
		t.Errorf("reset did not restore env value: %q", os.Getenv("REVIEW_LANGUAGE"))
	}
	if _, set := os.LookupEnv("REVIEW_MAX_FILES"); set {
		t.Error("reset should unset a value that was not in the environment")
	}
}

func TestSettingsValidationAndRestart(t *testing.T) {
	m, err := NewSettingsManager(openTestStore(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []map[string]string{
		{"REVIEW_MAX_FILES": "dużo"},
		{"REVIEW_ENGINE": "gpt"},
		{"REVIEW_POST_TO_GITLAB": "maybe"},
		{"JIRA_URL": "firma.atlassian.net"},
		{"JIRA_ISSUE_KEY_PATTERN": "([A-Z]+"},
		{"NOT_A_SETTING": "x"},
	} {
		if _, err := m.Update(bad, nil); err == nil {
			t.Errorf("expected validation error for %v", bad)
		}
	}
	t.Setenv("PORT", "8345")
	restart, err := m.Update(map[string]string{"PORT": "9000", "REVIEW_GITLAB_MODE": "comment"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(restart) != 1 || restart[0] != "PORT" {
		t.Errorf("restart = %v, want [PORT]", restart)
	}
	if _, err := m.Update(map[string]string{"GITLAB_PROJECTS": " grupa/api ,, grupa/front "}, nil); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("GITLAB_PROJECTS"); got != "grupa/api,grupa/front" {
		t.Errorf("list not normalised: %q", got)
	}
}

func TestStoreReviews(t *testing.T) {
	s := openTestStore(t)
	save := func(pid, iid int, verdict string) {
		data, _ := json.Marshal(CodeReview{Summary: verdict, Verdict: verdict, Score: 5})
		if err := s.SaveReview(&ReviewRecord{ProjectID: pid, IID: iid, Verdict: verdict, Score: 5, Review: data}); err != nil {
			t.Fatal(err)
		}
	}
	save(1, 10, "request_changes")
	save(1, 10, "approve") // newer review of the same MR
	save(1, 11, "approve_with_comments")
	save(2, 10, "approve")

	all, err := s.LatestReviews(0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("latest per MR = %d, want 3", len(all))
	}
	for _, r := range all {
		if r.ProjectID == 1 && r.IID == 10 && r.Verdict != "approve" {
			t.Errorf("latest review of 1/10 should be the newest, got %s", r.Verdict)
		}
	}
	proj1, _ := s.LatestReviews(1, 100)
	if len(proj1) != 2 {
		t.Errorf("project 1 latest = %d, want 2", len(proj1))
	}
	hist, _ := s.ReviewHistory(1, 10)
	if len(hist) != 2 || hist[0].Verdict != "approve" {
		t.Errorf("history = %+v", hist)
	}
	if n, _ := s.DeleteReviews(1, 10); n != 2 {
		t.Errorf("deleted %d, want 2", n)
	}
	if has, _ := s.HasReview(1, 10); has {
		t.Error("review still present after delete")
	}
}
