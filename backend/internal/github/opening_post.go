package github

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/octobud-hq/octobud/backend/internal/github/types"
)

func (c *clientImpl) FetchOpeningPost(
	ctx context.Context, owner, repo string, number int, discussion bool,
) (types.TimelineEvent, error) {
	const fields = `id body createdAt lastEditedAt url author { login __typename }`
	selection := `issueOrPullRequest(number: $number) {
		... on Issue { ` + fields + ` }
		... on PullRequest { ` + fields + ` }
	}`
	if discussion {
		selection = `discussion(number: $number) { ` + fields + ` }`
	}
	var data struct {
		Repository *struct {
			Post *struct {
				ID           string     `json:"id"`
				Body         string     `json:"body"`
				CreatedAt    *time.Time `json:"createdAt"`
				LastEditedAt *time.Time `json:"lastEditedAt"`
				URL          string     `json:"url"`
				Author       *struct {
					Login string `json:"login"`
					Type  string `json:"__typename"`
				} `json:"author"`
			} `json:"post"`
		} `json:"repository"`
	}
	err := c.fetchDiscussionGraphQL(ctx, GraphQLRequest{
		Query: `query($owner: String!, $repo: String!, $number: Int!) {
			repository(owner: $owner, name: $repo) { post: ` + selection + ` }
		}`,
		Variables: map[string]interface{}{"owner": owner, "repo": repo, "number": number},
	}, &data)
	if err != nil {
		return types.TimelineEvent{}, fmt.Errorf("fetch opening post: %w", err)
	}
	if data.Repository == nil || data.Repository.Post == nil || data.Repository.Post.ID == "" {
		return types.TimelineEvent{}, fmt.Errorf("github: opening post not found")
	}
	post := data.Repository.Post
	event := types.TimelineEvent{
		Event: "opened", ID: json.RawMessage(strconv.Quote(post.ID)), Body: post.Body,
		CreatedAt: post.CreatedAt, UpdatedAt: post.LastEditedAt, HTMLURL: post.URL,
	}
	if post.Author != nil {
		event.User = &types.SimpleUser{Login: post.Author.Login, Type: post.Author.Type}
	}
	return event, nil
}
