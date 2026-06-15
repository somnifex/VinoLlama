package conversations

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Conversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages"`
}

type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Summary struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Model        string    `json:"model"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	MessageCount int       `json:"message_count"`
}

type Store struct {
	Dir string
}

type CreateRequest struct {
	Title    string
	Model    string
	Messages []Message
}

type UpdateRequest struct {
	Title    *string
	Model    *string
	Messages []Message
}

func NewStore(dir string) (Store, error) {
	if strings.TrimSpace(dir) == "" {
		return Store{}, errors.New("conversation directory must not be empty")
	}
	return Store{Dir: dir}, nil
}

func (s Store) Init() error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("create conversation directory %s: %w", s.Dir, err)
	}
	return nil
}

func (s Store) Create(req CreateRequest) (Conversation, error) {
	if err := s.Init(); err != nil {
		return Conversation{}, err
	}
	now := time.Now().UTC()
	messages, err := normalizeMessages(req.Messages, now)
	if err != nil {
		return Conversation{}, err
	}
	id, err := newID()
	if err != nil {
		return Conversation{}, err
	}
	conv := Conversation{
		ID:        id,
		Title:     titleOrDefault(req.Title, messages),
		Model:     strings.TrimSpace(req.Model),
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  messages,
	}
	if err := s.Write(conv); err != nil {
		return Conversation{}, err
	}
	return conv, nil
}

func (s Store) List() ([]Summary, error) {
	if err := s.Init(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, fmt.Errorf("read conversation directory: %w", err)
	}
	var summaries []Summary
	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".json" {
			continue
		}
		conv, err := s.Read(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, Summary{
			ID:           conv.ID,
			Title:        conv.Title,
			Model:        conv.Model,
			CreatedAt:    conv.CreatedAt,
			UpdatedAt:    conv.UpdatedAt,
			MessageCount: len(conv.Messages),
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt)
	})
	return summaries, nil
}

func (s Store) Read(id string) (Conversation, error) {
	clean, err := CleanID(id)
	if err != nil {
		return Conversation{}, err
	}
	data, err := os.ReadFile(s.path(clean))
	if err != nil {
		return Conversation{}, fmt.Errorf("read conversation %s: %w", clean, err)
	}
	var conv Conversation
	if err := json.Unmarshal(data, &conv); err != nil {
		return Conversation{}, fmt.Errorf("decode conversation %s: %w", clean, err)
	}
	if _, err := CleanID(conv.ID); err != nil {
		return Conversation{}, err
	}
	return conv, nil
}

func (s Store) Update(id string, req UpdateRequest) (Conversation, error) {
	conv, err := s.Read(id)
	if err != nil {
		return Conversation{}, err
	}
	if req.Title != nil {
		conv.Title = strings.TrimSpace(*req.Title)
	}
	if req.Model != nil {
		conv.Model = strings.TrimSpace(*req.Model)
	}
	if req.Messages != nil {
		messages, err := normalizeMessages(req.Messages, time.Now().UTC())
		if err != nil {
			return Conversation{}, err
		}
		conv.Messages = messages
	}
	if strings.TrimSpace(conv.Title) == "" {
		conv.Title = titleOrDefault("", conv.Messages)
	}
	conv.UpdatedAt = time.Now().UTC()
	if err := s.Write(conv); err != nil {
		return Conversation{}, err
	}
	return conv, nil
}

func (s Store) Delete(id string) error {
	clean, err := CleanID(id)
	if err != nil {
		return err
	}
	if err := os.Remove(s.path(clean)); err != nil {
		return fmt.Errorf("delete conversation %s: %w", clean, err)
	}
	return nil
}

func (s Store) ExportMarkdown(id string) (string, error) {
	conv, err := s.Read(id)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", valueOrDefault(conv.Title, "Conversation"))
	if conv.Model != "" {
		fmt.Fprintf(&b, "Model: `%s`\n\n", conv.Model)
	}
	for _, msg := range conv.Messages {
		role := valueOrDefault(msg.Role, "message")
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", titleRole(role), msg.Content)
	}
	return b.String(), nil
}

func (s Store) Write(conv Conversation) error {
	clean, err := CleanID(conv.ID)
	if err != nil {
		return err
	}
	if err := s.Init(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(conv, "", "  ")
	if err != nil {
		return fmt.Errorf("encode conversation %s: %w", clean, err)
	}
	if err := os.WriteFile(s.path(clean), append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write conversation %s: %w", clean, err)
	}
	return nil
}

func CleanID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", errors.New("conversation id must not be empty")
	}
	if id == "." || id == ".." || strings.ContainsAny(id, `/\`) {
		return "", fmt.Errorf("conversation id %q must not contain path separators", id)
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return "", fmt.Errorf("conversation id %q contains unsupported character %q", id, r)
	}
	return id, nil
}

func (s Store) path(id string) string {
	return filepath.Join(s.Dir, id+".json")
}

func normalizeMessages(messages []Message, fallbackTime time.Time) ([]Message, error) {
	out := make([]Message, 0, len(messages))
	for _, msg := range messages {
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "user"
		}
		if role != "system" && role != "user" && role != "assistant" {
			return nil, fmt.Errorf("unsupported message role %q", role)
		}
		if msg.CreatedAt.IsZero() {
			msg.CreatedAt = fallbackTime
		}
		msg.Role = role
		msg.Content = strings.TrimSpace(msg.Content)
		out = append(out, msg)
	}
	return out, nil
}

func newID() (string, error) {
	var bytes [6]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("conv-%d-%s", time.Now().UTC().UnixNano(), hex.EncodeToString(bytes[:])), nil
}

func titleOrDefault(title string, messages []Message) string {
	title = strings.TrimSpace(title)
	if title != "" {
		return truncate(title, 80)
	}
	for _, msg := range messages {
		if msg.Role == "user" && strings.TrimSpace(msg.Content) != "" {
			return truncate(msg.Content, 80)
		}
	}
	return "New conversation"
}

func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return strings.TrimSpace(value[:max])
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func titleRole(role string) string {
	role = strings.TrimSpace(role)
	if role == "" {
		return "Message"
	}
	return strings.ToUpper(role[:1]) + role[1:]
}
