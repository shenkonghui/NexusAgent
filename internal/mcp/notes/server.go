package notesmcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"opennexus/internal/models"
	"opennexus/internal/repository"
)

// Handler 返回带 Bearer 鉴权的 Notes MCP Streamable HTTP Handler。
func Handler(noteRepo *repository.NoteRepository, settingsRepo *repository.NoteSettingsRepository) http.Handler {
	inner := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return newServer(noteRepo)
	}, &mcp.StreamableHTTPOptions{Stateless: true})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, err := BearerUserID(r, settingsRepo)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		inner.ServeHTTP(w, r.WithContext(withUserID(r.Context(), uid)))
	})
}

func newServer(noteRepo *repository.NoteRepository) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "opennexus-notes", Version: "1.0.0"}, nil)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_note_tags",
		Description: "列出当前用户全部笔记标签",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, listTagsOut, error) {
		uid, ok := userIDFrom(ctx)
		if !ok {
			return nil, listTagsOut{}, fmt.Errorf("未认证")
		}
		tags, err := noteRepo.ListTags(uid)
		if err != nil {
			return nil, listTagsOut{}, err
		}
		if tags == nil {
			tags = []string{}
		}
		return nil, listTagsOut{Tags: tags}, nil
	})
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_notes",
		Description: "按标签列出笔记；无标题时返回全文",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listNotesIn) (*mcp.CallToolResult, listNotesOut, error) {
		return handleListNotes(ctx, noteRepo, in)
	})
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_note",
		Description: "按 ID 获取单条笔记全文",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in getNoteIn) (*mcp.CallToolResult, noteFullOut, error) {
		return handleGetNote(ctx, noteRepo, in)
	})
	return srv
}

type listTagsOut struct {
	Tags []string `json:"tags" jsonschema:"标签列表"`
}

type listNotesIn struct {
	Tag string `json:"tag" jsonschema:"要过滤的标签"`
}

type listNotesOut struct {
	Notes []noteListItem `json:"notes"`
}

type noteListItem struct {
	ID        uint     `json:"id"`
	Title     string   `json:"title"`
	Tags      []string `json:"tags"`
	UpdatedAt string   `json:"updated_at"`
	Content   string   `json:"content,omitempty"`
}

type getNoteIn struct {
	ID uint `json:"id" jsonschema:"笔记 ID"`
}

type noteFullOut struct {
	ID        uint     `json:"id"`
	Title     string   `json:"title"`
	Content   string   `json:"content"`
	Tags      []string `json:"tags"`
	UpdatedAt string   `json:"updated_at"`
}

func handleListNotes(ctx context.Context, noteRepo *repository.NoteRepository, in listNotesIn) (*mcp.CallToolResult, listNotesOut, error) {
	uid, ok := userIDFrom(ctx)
	if !ok {
		return nil, listNotesOut{}, fmt.Errorf("未认证")
	}
	tag := strings.TrimSpace(in.Tag)
	if tag == "" {
		return nil, listNotesOut{}, fmt.Errorf("tag 必填")
	}
	list, err := noteRepo.FindByUserID(uid, tag)
	if err != nil {
		return nil, listNotesOut{}, err
	}
	items := make([]noteListItem, 0, len(list))
	for i := range list {
		items = append(items, toListItem(&list[i]))
	}
	return nil, listNotesOut{Notes: items}, nil
}

func handleGetNote(ctx context.Context, noteRepo *repository.NoteRepository, in getNoteIn) (*mcp.CallToolResult, noteFullOut, error) {
	uid, ok := userIDFrom(ctx)
	if !ok {
		return nil, noteFullOut{}, fmt.Errorf("未认证")
	}
	if in.ID == 0 {
		return nil, noteFullOut{}, fmt.Errorf("id 必填")
	}
	n, err := noteRepo.FindByID(in.ID)
	if err != nil || n.UserID != uid {
		return nil, noteFullOut{}, fmt.Errorf("未找到")
	}
	return nil, noteFullOut{
		ID:        n.ID,
		Title:     n.Title,
		Content:   n.Content,
		Tags:      parseTags(n.Tags),
		UpdatedAt: n.UpdatedAt.Format(time.RFC3339),
	}, nil
}

func toListItem(n *models.Note) noteListItem {
	item := noteListItem{
		ID:        n.ID,
		Title:     n.Title,
		Tags:      parseTags(n.Tags),
		UpdatedAt: n.UpdatedAt.Format(time.RFC3339),
	}
	if noTitle(n.Title) {
		item.Content = n.Content
	}
	return item
}

func noTitle(title string) bool {
	t := strings.TrimSpace(title)
	return t == "" || t == "无标题"
}

func parseTags(raw string) []string {
	if raw == "" {
		return []string{}
	}
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		return []string{}
	}
	return tags
}
