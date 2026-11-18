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

func TestFetchOpeningPostUsesBodyEditTime(t *testing.T) {
	for _, discussion := range []bool{false, true} {
		t.Run(map[bool]string{false: "issue or PR", true: "discussion"}[discussion], func(t *testing.T) {
			client := newTestClient("")
			client.httpClient.Transport = discussionTransport(func(request *http.Request) (*http.Response, error) {
				var payload GraphQLRequest
				require.NoError(t, json.NewDecoder(request.Body).Decode(&payload))
				require.Equal(t, "o", payload.Variables["owner"])
				require.Equal(t, "r", payload.Variables["repo"])
				require.EqualValues(t, 1, payload.Variables["number"])
				require.Contains(t, payload.Query, "lastEditedAt")
				require.NotContains(t, payload.Query, "updatedAt")
				if discussion {
					require.Contains(t, payload.Query, "post: discussion")
				} else {
					require.Contains(t, payload.Query, "post: issueOrPullRequest")
					require.Contains(t, payload.Query, "... on Issue")
					require.Contains(t, payload.Query, "... on PullRequest")
				}
				body := `{"data":{"repository":{"post":{
					"id":"post","body":"Opening **post**","createdAt":"2026-11-10T12:00:00Z",
					"lastEditedAt":"2026-11-11T12:00:00Z","url":"https://github.com/o/r/issues/1",
					"author":{"login":"author","__typename":"User"}
				}}}}`
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			event, err := client.FetchOpeningPost(t.Context(), "o", "r", 1, discussion)
			require.NoError(t, err)
			require.Equal(t, "opened", event.Event)
			require.Equal(t, json.RawMessage(`"post"`), event.ID)
			require.Equal(t, "Opening **post**", event.Body)
			require.Equal(t, "https://github.com/o/r/issues/1", event.HTMLURL)
			require.Equal(t, "User", event.User.Type)
			require.Equal(t, time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC), *event.CreatedAt)
			require.Equal(t, time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC), *event.UpdatedAt)
		})
	}
}

func TestFetchOpeningPostRejectsUnavailableEvidence(t *testing.T) {
	for _, body := range []string{
		`{"data":{"repository":null}}`,
		`{"data":{"repository":{"post":null}}}`,
		`{"data":{"repository":{"post":{}}}}`,
		`{"errors":[{"message":"denied"}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			client := newTestClient("")
			client.httpClient.Transport = discussionTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			_, err := client.FetchOpeningPost(t.Context(), "o", "r", 1, false)
			require.Error(t, err)
		})
	}
}

func TestFetchOpeningPostPreservesAnUneditedEmptyBody(t *testing.T) {
	client := newTestClient("")
	client.httpClient.Transport = discussionTransport(func(*http.Request) (*http.Response, error) {
		body := `{"data":{"repository":{"post":{
				"id":"post","body":"","createdAt":"2026-11-10T12:00:00Z",
				"lastEditedAt":null,"author":null,"url":"https://github.com/o/r/issues/1"
			}}}}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	event, err := client.FetchOpeningPost(t.Context(), "o", "r", 1, false)
	require.NoError(t, err)
	require.Empty(t, event.Body)
	require.Nil(t, event.UpdatedAt)
	require.Nil(t, event.User)
	require.NotNil(t, event.CreatedAt)
}
