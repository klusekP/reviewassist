package services

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
)

// ProjectContext is the knowledge about the reviewed repository shared by all per-file prompts.
type ProjectContext struct {
	Docs      string // project documentation (README, CLAUDE.md, docs/*.md ...)
	Structure string // list of repository paths
}

var docExtensions = map[string]bool{".md": true, ".rst": true, ".txt": true, ".adoc": true, "": true}

// BuildProjectContext reads documentation and repository structure at the MR head commit.
func (r *ReviewService) BuildProjectContext(ref string) *ProjectContext {
	cfg := r.config
	pc := &ProjectContext{}

	// --- documentation ---
	root, err := r.gitlabService.ListTree("", ref, false, 0)
	if err != nil {
		logrus.Warnf("[Context] Cannot list repository root: %v", err)
	}
	rootEntries := map[string]string{} // path -> type (blob/tree)
	for _, n := range root {
		rootEntries[n.Path] = n.Type
	}

	var docFiles []string
	for _, p := range cfg.DocsPaths {
		p = strings.Trim(p, "/")
		typ, inRoot := rootEntries[p]
		if !strings.Contains(p, "/") && !inRoot {
			continue // not present - skip without an API call
		}
		if typ == "tree" {
			nodes, err := r.gitlabService.ListTree(p, ref, true, 200)
			if err != nil {
				logrus.Warnf("[Context] Cannot list %s: %v", p, err)
				continue
			}
			var files []string
			for _, n := range nodes {
				if n.Type == "blob" && docExtensions[strings.ToLower(path.Ext(n.Path))] {
					files = append(files, n.Path)
				}
			}
			sort.Strings(files)
			docFiles = append(docFiles, files...)
		} else {
			docFiles = append(docFiles, p)
		}
	}

	var docs strings.Builder
	remaining := cfg.DocsMaxChars
	for _, f := range docFiles {
		if remaining <= 200 {
			logrus.Infof("[Context] Docs budget exhausted, skipped remaining files after %s", f)
			break
		}
		content, err := r.gitlabService.GetRawFile(f, ref)
		if err != nil {
			continue
		}
		content = truncate(strings.TrimSpace(content), remaining-100, "[file truncated]")
		block := fmt.Sprintf("--- %s ---\n%s\n\n", f, content)
		docs.WriteString(block)
		remaining -= len(block)
	}
	pc.Docs = docs.String()
	logrus.Infof("[Context] Loaded %d chars of documentation from %d candidate file(s)", len(pc.Docs), len(docFiles))

	// --- repository structure ---
	nodes, err := r.gitlabService.ListTree("", ref, true, cfg.TreeMaxPaths*3)
	if err != nil {
		logrus.Warnf("[Context] Cannot list repository tree: %v", err)
	}
	var paths []string
	for _, n := range nodes {
		if n.Type != "blob" || cfg.IsIgnored(n.Path) || strings.HasPrefix(path.Base(n.Path), ".") {
			continue
		}
		paths = append(paths, n.Path)
		if len(paths) >= cfg.TreeMaxPaths {
			paths = append(paths, "... (list truncated)")
			break
		}
	}
	pc.Structure = strings.Join(paths, "\n")
	return pc
}

// GetFileContent returns the full new version of a changed file (empty for deleted files).
func (r *ReviewService) GetFileContent(d Diff, ref string) string {
	if d.DeletedFile {
		return ""
	}
	content, err := r.gitlabService.GetRawFile(d.NewPath, ref)
	if err != nil {
		logrus.Warnf("[Context] Cannot read %s@%s: %v", d.NewPath, shortSha(ref), err)
		return ""
	}
	return content
}

// numberLines prefixes each line with its number, so the model can reference exact lines.
func numberLines(content string) string {
	lines := strings.Split(content, "\n")
	var b strings.Builder
	for i, l := range lines {
		fmt.Fprintf(&b, "%5d  %s\n", i+1, l)
	}
	return b.String()
}

func shortSha(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
