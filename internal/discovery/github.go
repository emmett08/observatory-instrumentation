package discovery

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/emmett08/observatory-instrumentation/internal/config"
)

type issue struct {
	Number     int64  `json:"number"`
	URL        string `json:"url"`
	Repository struct {
		Name string `json:"nameWithOwner"`
		ID   int64  `json:"databaseId"`
	} `json:"repository"`
}
type pullEvidence struct {
	Repository     string   `json:"repository"`
	Number         int64    `json:"number"`
	URL            string   `json:"url"`
	HeadBranch     string   `json:"head_branch"`
	HeadRepository string   `json:"head_repository"`
	ClosingIssues  []issue  `json:"closing_issues"`
	ResponseSHA256 []string `json:"response_sha256"`
	Source         string   `json:"source"`
}

const query = `query($owner:String!,$name:String!,$number:Int!,$cursor:String){repository(owner:$owner,name:$name){nameWithOwner databaseId pullRequest(number:$number){number url headRefName headRepository{nameWithOwner} closingIssuesReferences(first:100,after:$cursor){totalCount nodes{number url repository{nameWithOwner databaseId}} pageInfo{hasNextPage endCursor}}}}}`

func fetchPR(client api, repository string, number int64) (pullEvidence, error) {
	result := pullEvidence{Repository: repository, Number: number, ClosingIssues: []issue{}, ResponseSHA256: []string{}, Source: "github_graphql_closing_issues"}
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || number <= 0 {
		return result, errors.New("invalid PR identity")
	}
	var cursor any
	seen := map[string]bool{}
	cursors := map[string]bool{}
	expectedTotal := -1
	var expectedRepositoryID int64
	for page := 0; page < 10; page++ {
		var response struct {
			Errors []any `json:"errors"`
			Data   struct {
				Repository *struct {
					Name string `json:"nameWithOwner"`
					ID   int64  `json:"databaseId"`
					PR   *struct {
						Number int64  `json:"number"`
						URL    string `json:"url"`
						Branch string `json:"headRefName"`
						Head   *struct {
							Name string `json:"nameWithOwner"`
						} `json:"headRepository"`
						Closing struct {
							Total int     `json:"totalCount"`
							Nodes []issue `json:"nodes"`
							Page  struct {
								More   bool   `json:"hasNextPage"`
								Cursor string `json:"endCursor"`
							} `json:"pageInfo"`
						} `json:"closingIssuesReferences"`
					} `json:"pullRequest"`
				} `json:"repository"`
			} `json:"data"`
		}
		hash, err := client.graphql(map[string]any{"query": query, "variables": map[string]any{"owner": parts[0], "name": parts[1], "number": number, "cursor": cursor}}, &response)
		if err != nil {
			return result, err
		}
		if len(response.Errors) > 0 || response.Data.Repository == nil {
			return result, errors.New("PR link query is incomplete")
		}
		r := response.Data.Repository
		pr := r.PR
		if !strings.EqualFold(r.Name, repository) || r.ID <= 0 || pr == nil || pr.Number != number || pr.URL != fmt.Sprintf("https://github.com/%s/pull/%d", r.Name, number) {
			return result, errors.New("PR link response identity mismatch")
		}
		if page == 0 {
			expectedRepositoryID = r.ID
		} else if expectedRepositoryID != r.ID {
			return result, errors.New("repository identity changed during pagination")
		}
		if len(pr.Branch) > 1024 || strings.ContainsAny(pr.Branch, "\r\n\t") || (pr.Head != nil && !repositoryPattern.MatchString(pr.Head.Name)) {
			return result, errors.New("invalid PR branch identity")
		}
		if expectedTotal < 0 {
			expectedTotal = pr.Closing.Total
		} else if expectedTotal != pr.Closing.Total {
			return result, errors.New("PR closing issues changed during pagination")
		}
		if expectedTotal < 0 || expectedTotal > 1000 || len(pr.Closing.Nodes) > 100 {
			return result, errors.New("PR closing issue limit exceeded")
		}
		if page == 0 {
			result.URL = pr.URL
			result.HeadBranch = pr.Branch
			if pr.Head != nil {
				result.HeadRepository = pr.Head.Name
			}
		} else if result.HeadBranch != pr.Branch || (pr.Head != nil && result.HeadRepository != pr.Head.Name) {
			return result, errors.New("PR branch changed during pagination")
		}
		result.ResponseSHA256 = append(result.ResponseSHA256, hash)
		for _, item := range pr.Closing.Nodes {
			if item.Number <= 0 || item.Repository.ID <= 0 || !repositoryPattern.MatchString(item.Repository.Name) || item.URL != fmt.Sprintf("https://github.com/%s/issues/%d", item.Repository.Name, item.Number) {
				return result, errors.New("invalid closing issue identity")
			}
			if strings.EqualFold(item.Repository.Name, r.Name) && item.Repository.ID != r.ID {
				return result, errors.New("closing issue repository ID disagrees with repository metadata")
			}
			// Issue numbers only identify an issue within its repository.
			key := fmt.Sprintf("%d/%d", item.Repository.ID, item.Number)
			if seen[key] {
				return result, errors.New("duplicate closing issue page")
			}
			seen[key] = true
			result.ClosingIssues = append(result.ClosingIssues, item)
		}
		if !pr.Closing.Page.More {
			if len(result.ClosingIssues) != expectedTotal {
				return result, errors.New("incomplete closing issue pages")
			}
			return result, nil
		}
		if pr.Closing.Page.Cursor == "" || cursors[pr.Closing.Page.Cursor] {
			return result, errors.New("invalid closing issue pagination")
		}
		cursors[pr.Closing.Page.Cursor] = true
		cursor = pr.Closing.Page.Cursor
	}
	return result, errors.New("closing issue pagination limit reached")
}

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
var shaPattern = regexp.MustCompile(`^[0-9a-fA-F]{40,64}$`)

func issueTask(cfg *config.Config, repository string, evidence pullEvidence) (string, string, *issue) {
	if len(evidence.ClosingIssues) == 0 {
		return "", "no_closing_issue", nil
	}
	if len(evidence.ClosingIssues) != 1 {
		return "", "multiple_closing_issues", nil
	}
	i := evidence.ClosingIssues[0]
	if !strings.EqualFold(i.Repository.Name, repository) {
		return "", "cross_repository_issue", nil
	}
	return cfg.TaskID(repository, i.Repository.ID, i.Number), "unambiguous_closing_issue", &i
}

func commitPRs(client api, repository, sha string) ([]int64, []string, error) {
	if !shaPattern.MatchString(sha) {
		return nil, nil, errors.New("run has no valid commit SHA for PR lookup")
	}
	result := []int64{}
	hashes := []string{}
	seen := map[int64]bool{}
	for page := 1; page <= 10; page++ {
		var items []struct {
			Number int64 `json:"number"`
			Base   struct {
				Repo struct {
					Name string `json:"full_name"`
				} `json:"repo"`
			} `json:"base"`
		}
		endpoint := fmt.Sprintf("repos/%s/commits/%s/pulls?per_page=100&page=%d", repository, url.PathEscape(sha), page)
		hash, err := client.get(endpoint, &items)
		if err != nil {
			return result, hashes, err
		}
		hashes = append(hashes, hash)
		if len(items) > 100 {
			return result, hashes, errors.New("invalid associated PR page")
		}
		for _, item := range items {
			if item.Number <= 0 || !strings.EqualFold(item.Base.Repo.Name, repository) {
				return result, hashes, errors.New("associated PR identity mismatch")
			}
			if !seen[item.Number] {
				seen[item.Number] = true
				result = append(result, item.Number)
			}
		}
		if len(items) < 100 {
			sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
			return result, hashes, nil
		}
	}
	return result, hashes, errors.New("associated PR pagination limit reached")
}
