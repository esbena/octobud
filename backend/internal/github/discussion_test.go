package github

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type discussionTransport func(*http.Request) (*http.Response, error)

func (transport discussionTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestDiscussionCommentAndReplyPaginationMetadata(t *testing.T) {
	client := newTestClient("")
	requests := 0
	client.httpClient.Transport = discussionTransport(func(request *http.Request) (*http.Response, error) {
		var payload GraphQLRequest
		require.NoError(t, json.NewDecoder(request.Body).Decode(&payload))
		require.Contains(t, payload.Query, "__typename")
		require.Contains(t, payload.Query, "startCursor")
		requests++
		body := ""
		if requests == 1 {
			require.Equal(t, "previous-comments", payload.Variables["before"])
			require.Contains(t, payload.Query, "replies(last: 5)")
			body = `{"data":{"repository":{"discussion":{"comments":{
				"nodes":[{"id":"parent","createdAt":"2026-11-10T12:00:00Z",
					"author":{"login":"bot","__typename":"Bot"},"body":"root",
					"replies":{"totalCount":2,"nodes":[{"id":"recent",
						"createdAt":"2026-11-11T12:00:00Z","updatedAt":"2026-11-11T13:00:00Z",
						"author":{"login":"human","__typename":"User"},
						"body":"new","url":"https://github.com/o/r/discussions/1#recent"}],
						"pageInfo":{"hasPreviousPage":true,"startCursor":"recent-cursor"}}}],
				"pageInfo":{"hasPreviousPage":true,"startCursor":"parent-cursor"}
			}}}}}`
		} else {
			require.Equal(t, "parent", payload.Variables["id"])
			require.Equal(t, "recent-cursor", payload.Variables["before"])
			require.EqualValues(t, 100, payload.Variables["last"])
			body = `{"data":{"node":{"id":"parent","replies":{
				"nodes":[{"id":"older","createdAt":"2026-11-10T12:00:00Z","author":null,"body":"old"}],
				"pageInfo":{"hasPreviousPage":false,"startCursor":"old-cursor"}
			}}}}`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	events, more, cursor, err := client.FetchDiscussionComments(t.Context(), "o", "r", 1, 100, "previous-comments")
	require.NoError(t, err)
	require.True(t, more)
	require.Equal(t, "parent-cursor", cursor)
	require.Len(t, events, 1)
	require.Equal(t, "Bot", events[0].User.Type)
	require.Equal(t, 2, events[0].ReplyCount)
	require.True(t, events[0].HasMoreReplies)
	require.Equal(t, "recent-cursor", events[0].RepliesCursor)
	require.Len(t, events[0].Replies, 1)
	require.Equal(t, "User", events[0].Replies[0].Author.Type)
	require.Equal(t, "https://github.com/o/r/discussions/1#recent", events[0].Replies[0].HTMLURL)
	require.Equal(t, time.Date(2026, 11, 11, 13, 0, 0, 0, time.UTC), *events[0].Replies[0].UpdatedAt)

	replies, more, cursor, err := client.FetchDiscussionReplies(t.Context(), "parent", 100, events[0].RepliesCursor)
	require.NoError(t, err)
	require.False(t, more)
	require.Equal(t, "old-cursor", cursor)
	require.Len(t, replies, 1)
	require.Equal(t, "older", replies[0].ID)
	require.Empty(t, replies[0].Author.Login)
	require.Equal(t, 2, requests)
}

func TestDiscussionFetchRejectsUnavailableOrFailedGraphQL(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		body   string
	}{
		{"missing record", 200, `{"data":{"repository":{"discussion":null},"node":null}}`},
		{"wrong node type", 200, `{"data":{"repository":{"discussion":null},"node":{}}}`},
		{"graphql error", 200, `{"errors":[{"message":"denied"}]}`},
		{"http error", 403, `denied`},
		{"malformed response", 200, `{`},
		{"malformed data", 200, `{"data":[]}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient("")
			client.httpClient.Transport = discussionTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(tt.body))}, nil
			})
			_, _, _, err := client.FetchDiscussionComments(t.Context(), "o", "r", 1, 100, "")
			require.Error(t, err)
			_, _, _, err = client.FetchDiscussionReplies(t.Context(), "missing", 100, "cursor")
			require.Error(t, err)
		})
	}
}
