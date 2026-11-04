package services

import (
	"fmt"
	"strings"

	"codereviewAI/services/ai"
)

// securityChecklist is appended to every review prompt (agent and LLM engines).
const securityChecklist = `
## Security review (mandatory - check every item against the changed code)
1. Access control: every new/changed endpoint, API Platform operation, controller action, command or message handler has authentication and authorization (security attribute, is_granted, voter, middleware). Check object-level access: can user A read/modify user B's data by changing an ID (IDOR)? Are admin-only operations protected?
2. Injection: SQL/DQL/NoSQL built by string concatenation or interpolation, raw queries without parameters, shell commands, eval, template injection, LDAP/XPath, header injection.
3. Output encoding / XSS: unescaped output (Twig |raw, innerHTML, v-html, dangerouslySetInnerHTML), user data in JS/HTML/URL contexts.
4. Mass assignment / data exposure: serialization or denormalization groups, DTOs and forms that allow writing fields the user must not set (role, price, owner, status) or expose sensitive fields (password hash, tokens, emails, internal IDs, PII).
5. Input validation: missing validation constraints, type and range checks, enum checks; trust in client-side values (prices, totals, user IDs).
6. Secrets: hard-coded passwords, API keys, tokens, private keys or credentials in code, config, fixtures or tests; secrets or PII written to logs or error messages.
7. Files and network: path traversal in file paths, unrestricted file upload (type, size, storage location), SSRF (server-side requests to user-supplied URLs), open redirects, XXE / unsafe XML or YAML parsing, unsafe deserialization (unserialize, pickle, gob from untrusted input).
8. Authentication and sessions: CSRF protection on state-changing forms, CORS configuration, cookie flags, JWT validation, password handling and hashing, weak randomness (rand instead of crypto-secure) for tokens.
9. Abuse and availability: unbounded queries or collections (pagination disabled, no limit), N+1 on public endpoints, missing rate limiting on login/expensive operations, regex DoS, large payloads.
10. Data integrity: race conditions and missing transactions/locks in money, stock or status changes; missing DB constraints; destructive migrations without safeguards.
11. Dependencies: newly added packages or version changes - flag only known-vulnerable, abandoned or suspicious packages (typosquatting).

For every security finding: use type "security", describe the concrete attack scenario (who can do what, how, and the impact) and give a concrete fix.
Report only issues that are actually present in the changed code or directly caused by it. Do not report theoretical issues already handled elsewhere in the project - check before reporting.
`

// qualityChecklist covers code volume, refactoring, performance, data structures and algorithms.
const qualityChecklist = `
## Code volume, refactoring and performance (mandatory - check against the changed code)
Code volume and simplicity:
1. Is there more code than the task needs? Look for over-engineering (abstractions, interfaces, factories or config options with a single use), speculative "future" code, dead or unreachable code, commented-out code, debug leftovers.
2. Duplication: copy-pasted blocks inside the MR, and new code that re-implements something that ALREADY EXISTS in the project (search the codebase for similar helpers, services, repository methods, validators, utilities before reporting). Point to the existing code to reuse.
3. Could the same behaviour be written noticeably shorter and clearer (standard library / framework feature instead of hand-written code, early returns instead of deep nesting, a simpler data flow)? Show the shorter version.

Refactoring (only for code touched by this MR):
4. Functions/methods that are too long or do several things, classes with mixed responsibilities, deep nesting, long parameter lists, boolean flag parameters, feature envy, magic numbers/strings, unclear names.
5. Wrong layer: business logic in controllers/templates, queries outside repositories, framework code leaking into domain - judged by the project's own architecture and docs.
6. Propose concrete refactorings with a short before/after code example. Report only refactorings with a clear benefit; at most the 5 most valuable refactoring/simplification findings per MR.

Performance, data structures and algorithms:
7. Algorithmic complexity: nested loops over collections (O(n^2) or worse) where a map/set/index gives O(n); repeated linear searches (in_array, array_search, slices.Contains, list.indexOf inside loops); sorting or copying inside loops; string concatenation in loops.
8. Data structures: list/array used for membership tests or lookups instead of a hash map/set; wrong collection for the access pattern; unnecessary conversions and copies of large collections.
9. Database and I/O: queries or HTTP/API calls inside loops (N+1), loading whole entities or tables when a count, exists, or selected columns are enough, missing pagination/limits, filtering or aggregating in code instead of in the query, missing index for a new query pattern, missing eager loading/joins, repeated identical calls that could be cached or batched.
10. Memory and resources: loading large datasets into memory instead of streaming/iterating, unclosed resources, unbounded caches or goroutines/workers, heavy work done on every request that could be done once.
Judge performance by realistic data sizes and how often the code runs (request path, loop, cron, one-off command) - check the call sites before reporting. Use type "performance" for performance findings and "refactoring" for volume/refactoring findings.
`

// requirementsGuide asks for an explicit checklist of task requirements with evidence in code.
const requirementsGuide = `
## Task requirements checklist (mandatory when a task or MR description states requirements)
- Extract the requirements from the Jira task: description, acceptance criteria, and decisions in comments (a later comment overrides an earlier one - use the final agreed version). Include subtasks only if they are part of this MR's scope. If no Jira task is linked, use concrete requirements stated in the MR description; if there are none, return an empty list.
- Each requirement must be atomic and verifiable (split "list, filter and export" into three). Paraphrase briefly in the review language. At most 15 requirements; merge trivial ones.
- For each requirement, verify it in the code (follow the flow: endpoint/route -> controller/handler -> service -> query/entity -> response/template), then set "status":
  done = fully implemented; partial = implemented but incomplete or with a deviation from the task (say what is missing); missing = not implemented in this MR; unclear = cannot be judged from code (e.g. depends on an attachment, a manual setting, or data).
- "evidence": for done/partial - the file:line where it is implemented (and what is missing for partial); for missing - where it would be expected; for unclear - why.
- Every missing or partial requirement must ALSO be reported as an issue of type "requirement" (missing: high; partial: medium or high depending on impact). Attachments listed in the task cannot be read - mark requirements that depend on them as unclear instead of guessing.
`

// severityGuide calibrates severities so the verdict is meaningful.
const severityGuide = `
## Severity calibration
- critical: exploitable without authentication or by any user with severe impact (data leak, account takeover, RCE, payment/price manipulation), or data loss/corruption in normal use.
- high: security issue exploitable by an authenticated user (IDOR, privilege escalation, sensitive data exposure), a bug that breaks the main flow of the task, or a requirement from the task that is not implemented.
- medium: bug in an edge case, missing validation/error handling with limited impact, missing tests for new business logic, clear violation of project conventions, a performance problem that will be noticeable with production data (N+1, O(n^2) on growing collections, unbounded query) - use high if it is on a hot request path and will clearly degrade the service, duplicated logic that already exists in the project.
- low: readability, naming, simplification and refactoring suggestions, minor performance improvements, defence in depth.
`

// verdictGuide asks for an explicit "is everything OK" decision.
const verdictGuide = `
## Final verdict (mandatory)
- "verdict": "request_changes" if there is any critical or high issue, or any requirement is missing; "approve_with_comments" if only medium/low issues; "approve" if you found nothing worth changing.
- "security_summary": 1-4 sentences stating explicitly whether the change is secure, which security areas were relevant for this change and what you verified (e.g. access control of the new endpoint, exposed fields, input validation). Write it even if no security issues were found.
- "quality_summary": 1-4 sentences assessing code volume (is there unnecessary code or duplication), structure/refactoring needs, and performance (complexity, data structures, queries) of the change. Write it even if everything is fine.
`

const reviewGuidelines = securityChecklist + qualityChecklist + requirementsGuide + severityGuide + verdictGuide

const (
	VerdictApprove             = "approve"
	VerdictApproveWithComments = "approve_with_comments"
	VerdictRequestChanges      = "request_changes"
)

var verdictRank = map[string]int{VerdictApprove: 1, VerdictApproveWithComments: 2, VerdictRequestChanges: 3}

// finalVerdict never lets the verdict be milder than the reported issues and requirements justify;
// the model can still be stricter.
func finalVerdict(modelVerdict string, issues []ai.Issue, reqs []ai.Requirement) string {
	computed := VerdictApprove
	for _, rq := range reqs {
		switch strings.ToLower(rq.Status) {
		case "missing":
			computed = VerdictRequestChanges
		case "partial":
			if verdictRank[computed] < verdictRank[VerdictApproveWithComments] {
				computed = VerdictApproveWithComments
			}
		}
	}
	for _, is := range issues {
		switch strings.ToLower(is.Severity) {
		case "critical", "high":
			computed = VerdictRequestChanges
		case "medium", "low":
			if verdictRank[computed] < verdictRank[VerdictApproveWithComments] {
				computed = VerdictApproveWithComments
			}
		}
	}
	model := strings.ToLower(strings.TrimSpace(modelVerdict))
	if verdictRank[model] > verdictRank[computed] {
		return model
	}
	return computed
}

// requirementsLine is a one-line status of the requirements checklist, e.g. "Wymagania: 3/5 zrealizowane, 1 częściowo, 1 brak".
func requirementsLine(reqs []ai.Requirement) string {
	if len(reqs) == 0 {
		return ""
	}
	counts := map[string]int{}
	for _, rq := range reqs {
		counts[strings.ToLower(rq.Status)]++
	}
	line := fmt.Sprintf("Wymagania: %d/%d zrealizowane", counts["done"], len(reqs))
	for _, st := range []struct{ key, label string }{{"partial", "częściowo"}, {"missing", "brak"}, {"unclear", "do weryfikacji"}} {
		if counts[st.key] > 0 {
			line += fmt.Sprintf(", %d %s", counts[st.key], st.label)
		}
	}
	return line
}
