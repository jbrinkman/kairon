package github

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// openIssuesQuery reads one page of open issues carrying a label, oldest first,
// with the labels and the oldest listCommentCap comments of each issue. It uses
// the `issues` connection, which reads the database. The repository, label,
// assignee and cursor are variables, never part of this text. $assignee is
// null when the -f flag is omitted, which filterBy treats as "any assignee".
var openIssuesQuery = fmt.Sprintf(`query($owner: String!, $name: String!, $label: String!, $assignee: String, $after: String) {
  repository(owner: $owner, name: $name) {
    issues(first: %d, after: $after, states: [OPEN], labels: [$label],
           filterBy: {assignee: $assignee},
           orderBy: {field: CREATED_AT, direction: ASC}) {
      pageInfo { hasNextPage endCursor }
      nodes {
        number title body
        labels(first: 100) { nodes { name } }
        comments(first: %d) { nodes { author { login } body createdAt } }
      }
    }
  }
}`, listPageSize, listCommentCap)

// issuePage is the decoded response of openIssuesQuery. Pointers tell a null or
// missing object apart from an empty one.
type issuePage struct {
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Data *struct {
		Repository *struct {
			Issues *struct {
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []*issueNode `json:"nodes"`
			} `json:"issues"`
		} `json:"repository"`
	} `json:"data"`
}

type issueNode struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	Labels struct {
		Nodes []Label `json:"nodes"`
	} `json:"labels"`
	Comments struct {
		Nodes []struct {
			Author *struct {
				Login string `json:"login"`
			} `json:"author"`
			Body      string    `json:"body"`
			CreatedAt time.Time `json:"createdAt"`
		} `json:"nodes"`
	} `json:"comments"`
}

// toRaw converts a GraphQL node into the rawIssue the rest of selectIssues
// uses. A null comment author (a deleted account) becomes an empty login.
func (n *issueNode) toRaw() rawIssue {
	ri := rawIssue{Issue: Issue{Number: n.Number, Title: n.Title, Body: n.Body, Labels: n.Labels.Nodes}}
	for _, c := range n.Comments.Nodes {
		rc := rawComment{Body: c.Body, CreatedAt: c.CreatedAt}
		if c.Author != nil {
			rc.Author.Login = c.Author.Login
		}
		ri.Comments = append(ri.Comments, rc)
	}
	return ri
}

// fetchOpenIssues returns every open issue labeled label (and assigned to
// assignee when non-empty), ascending by issue number. Any failure returns
// (nil, err): the caller never sees the pages that did succeed.
//
// It makes one `gh api graphql` call per page and follows pageInfo.endCursor in
// this loop rather than using `gh --paginate`, so a failed page stops the fetch
// and nothing from earlier pages is returned. It never reads the search index.
func fetchOpenIssues(run ghFunc, repo, label, assignee string) ([]rawIssue, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" {
		return nil, fmt.Errorf("repo %q must be in owner/name form", repo)
	}

	var (
		issues []rawIssue
		cursor string
		seen   = map[string]bool{}
	)
	for page := 1; ; page++ {
		// -f (string), never -F, so a value starting with "@" is not read as a file.
		args := []string{"api", "graphql", "-f", "query=" + openIssuesQuery,
			"-f", "owner=" + owner, "-f", "name=" + name, "-f", "label=" + label}
		if assignee != "" {
			args = append(args, "-f", "assignee="+assignee)
		}
		if cursor != "" {
			args = append(args, "-f", "after="+cursor)
		}
		out, err := run("", args...)
		if err != nil {
			if isRateLimited(err) {
				return nil, ErrRateLimited
			}
			return nil, fmt.Errorf("gh api graphql failed on page %d: %w", page, err)
		}

		var p issuePage
		if err := json.Unmarshal(out, &p); err != nil {
			return nil, fmt.Errorf("failed to parse gh output on page %d: %w", page, err)
		}
		if len(p.Errors) > 0 {
			return nil, fmt.Errorf("GraphQL error on page %d: %s", page, p.Errors[0].Message)
		}
		if p.Data == nil || p.Data.Repository == nil {
			return nil, fmt.Errorf("repository %s not found on page %d", repo, page)
		}
		conn := p.Data.Repository.Issues
		if conn == nil {
			return nil, fmt.Errorf("no issues connection in the response on page %d", page)
		}
		for _, n := range conn.Nodes {
			if n == nil {
				// A null node is an issue we cannot see. Dropping it would make
				// the caller treat it as gone, so fail the whole fetch instead.
				return nil, fmt.Errorf("null issue node on page %d", page)
			}
			issues = append(issues, n.toRaw())
		}

		if !conn.PageInfo.HasNextPage {
			break
		}
		next := conn.PageInfo.EndCursor
		if next == "" {
			return nil, fmt.Errorf("page %d has a next page but no end cursor", page)
		}
		if seen[next] {
			return nil, fmt.Errorf("end cursor %q repeated on page %d: the cursor does not advance", next, page)
		}
		seen[next] = true
		cursor = next
	}

	// The API returns CREATED_AT ASC, which keeps cursor paging stable. Sorting
	// by number here guarantees the ascending contract whatever order the API
	// (or a fake) returns.
	sort.SliceStable(issues, func(i, j int) bool { return issues[i].Number < issues[j].Number })
	return issues, nil
}
