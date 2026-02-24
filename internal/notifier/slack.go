package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/amishk599/firstin/internal/model"
)

// Ensure SlackNotifier implements model.Notifier.
var _ model.Notifier = (*SlackNotifier)(nil)

// SlackNotifier sends job alerts to a Slack channel via Incoming Webhooks.
type SlackNotifier struct {
	webhookURL string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewSlackNotifier returns a notifier that posts each job to Slack via webhook.
func NewSlackNotifier(webhookURL string, httpClient *http.Client, logger *slog.Logger) *SlackNotifier {
	return &SlackNotifier{
		webhookURL: webhookURL,
		httpClient: httpClient,
		logger:     logger,
	}
}

// Notify sends each job as a separate Slack message using Block Kit.
// Returns an error only if ALL messages fail. Individual failures are logged.
func (s *SlackNotifier) Notify(jobs []model.Job) error {
	if len(jobs) == 0 {
		return nil
	}

	failures := 0
	for i, j := range jobs {
		if i > 0 {
			time.Sleep(500 * time.Millisecond)
		}

		if err := s.sendMessage(j); err != nil {
			s.logger.Error("slack notification failed", "company", j.Company, "title", j.Title, "error", err)
			failures++
		}
	}

	sent := len(jobs) - failures
	if failures == len(jobs) {
		return fmt.Errorf("all %d slack notifications failed", failures)
	}
	s.logger.Info("slack notifications complete", "sent", sent, "failed", failures)
	return nil
}

func (s *SlackNotifier) sendMessage(j model.Job) error {
	payload := buildPayload(j)

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal slack payload: %w", err)
	}

	resp, err := s.httpClient.Post(s.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("post to slack: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := resp.Header.Get("Retry-After")
		secs, _ := strconv.Atoi(retryAfter)
		if secs <= 0 {
			secs = 1
		}
		s.logger.Warn("slack rate limited, retrying", "retry_after_secs", secs)
		time.Sleep(time.Duration(secs) * time.Second)

		resp2, err := s.httpClient.Post(s.webhookURL, "application/json", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("post to slack (retry): %w", err)
		}
		defer resp2.Body.Close()

		if resp2.StatusCode != http.StatusOK {
			return fmt.Errorf("slack returned %d on retry", resp2.StatusCode)
		}
		s.logger.Info("slack message sent", "company", j.Company, "title", j.Title, "retried", true)
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack returned %d", resp.StatusCode)
	}
	s.logger.Info("slack message sent", "company", j.Company, "title", j.Title)
	return nil
}

// Block Kit payload types.

type slackPayload struct {
	Blocks []slackBlock `json:"blocks"`
}

type slackBlock struct {
	Type     string         `json:"type"`
	Text     *slackText     `json:"text,omitempty"`
	Fields   []slackText    `json:"fields,omitempty"`
	Elements []slackElement `json:"elements,omitempty"`
}

type slackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type slackElement struct {
	Type  string    `json:"type"`
	Text  slackText `json:"text"`
	URL   string    `json:"url"`
	Style string    `json:"style"`
}

// SendTestMessage sends a dummy job notification to verify the integration works.
func SendTestMessage(n model.Notifier) error {
	now := time.Now()
	testJob := model.Job{
		ID:        "test-001",
		Company:   "FirstIn Test",
		Title:     "Test Notification — Integration Verified",
		Location:  "Everywhere",
		URL:       "https://www.ycombinator.com/jobs",
		PostedAt:  &now,
		FirstSeen: now,
		Source:    "test",
	}
	return n.Notify([]model.Job{testJob})
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func buildPayload(j model.Job) slackPayload {
	postedText := "Just detected"
	if j.PostedAt != nil {
		pst, err := time.LoadLocation("America/Los_Angeles")
		if err == nil {
			postedText = j.PostedAt.In(pst).Format(time.RFC1123)
		} else {
			postedText = j.PostedAt.Format(time.RFC1123)
		}
	}

	company := capitalize(j.Company)
	source := capitalize(j.Source)

	blocks := []slackBlock{
		{
			Type: "header",
			Text: &slackText{Type: "plain_text", Text: "🚀 " + company + ": " + j.Title},
		},
		{
			Type: "section",
			Fields: []slackText{
				{Type: "mrkdwn", Text: "*Company:*\n" + company},
				{Type: "mrkdwn", Text: "*Location:*\n" + j.Location},
			},
		},
		{
			Type: "section",
			Fields: []slackText{
				{Type: "mrkdwn", Text: "*Posted:*\n" + postedText},
				{Type: "mrkdwn", Text: "*Source:*\n" + source},
			},
		},
	}

	if j.Insights != nil {
		stack := strings.Join(j.Insights.TechStack, ", ")
		insightsText := fmt.Sprintf("*Role:* %s   *Exp:* %s   *Stack:* %s\n• %s\n• %s\n• %s",
			j.Insights.RoleType,
			j.Insights.YearsExp,
			stack,
			j.Insights.KeyPoints[0],
			j.Insights.KeyPoints[1],
			j.Insights.KeyPoints[2],
		)
		blocks = append(blocks, slackBlock{
			Type: "section",
			Text: &slackText{Type: "mrkdwn", Text: insightsText},
		})
	}

	blocks = append(blocks,
		slackBlock{
			Type: "actions",
			Elements: []slackElement{
				{
					Type:  "button",
					Text:  slackText{Type: "plain_text", Text: "Apply Now"},
					URL:   j.URL,
					Style: "primary",
				},
			},
		},
		slackBlock{Type: "divider"},
	)

	return slackPayload{Blocks: blocks}
}
