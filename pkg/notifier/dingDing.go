package notifier

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// DingDing holds the hook URL
type DingDing struct {
	URL       string // dingDing webhook url
	Usernames []string
	Channel   string // msgtype
}

// DingDingload holds the channel and attachments
/*type DingDingPayload struct {
	Channel     string            `json:"msgtype"`   // 对应JSON的channel
	Usernames   []string          `json:"atMobiles"`
	IconUrl     string            `json:"icon_url"`
	IconEmoji   string            `json:"icon_emoji"`
	Text        string            `json:"text,omitempty"`
	Attachments []DingDingAttachment `json:"attachments,omitempty"`
}*/

type DingDingPayload struct {
	Msgtype string       `json:"msgtype"`
	Text    DingDingText `json:"text"`
	At      DingDingAt   `json:"at"`
}

type DingDingText struct {
	Content string `json:"content"`
}

type DingDingAt struct {
	AtMobiles []string `json:"atMobiles"`
	IsAtAll   bool     `json:"isAtAll"`
}

// DingDingAttachment holds the markdown message body
type DingDingAttachment struct {
	AuthorName string          `json:"author_name"`
	Text       string          `json:"text"`
	MrkdwnIn   []string        `json:"mrkdwn_in"`
	Fields     []DingDingField `json:"fields"`
}

type DingDingField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// NewDingDing validates the DingDing URL and returns a DingDing object
func NewDingDing(hookURL string, username string, channel string) (*DingDing, error) {
	_, err := url.ParseRequestURI(hookURL)
	if err != nil {
		return nil, fmt.Errorf("invalid DingDing hook URL %s", hookURL)
	}

	if username == "" {
		return nil, errors.New("empty DingDing username")
	}

	usernames := strings.Split(username, ",")

	if channel == "" {
		return nil, errors.New("empty DingDing channel")
	}
	// 需要@人员的手机号码，多个使用逗号分隔
	return &DingDing{
		Channel:   channel,
		URL:       hookURL,
		Usernames: usernames,
	}, nil
}

// Post DingDing message
func (d *DingDing) Post(workload string, namespace string, message string, fields []Field, severity string) error {

	dingDingAt := DingDingAt{
		AtMobiles: d.Usernames,
		IsAtAll:   false,
	}

	payload := DingDingPayload{
		Msgtype: d.Channel,
		At:      dingDingAt,
	}

	dfields := make([]DingDingField, 0, len(fields))
	for _, f := range fields {
		dfields = append(dfields, DingDingField{f.Name, f.Value, false})
	}

	a := DingDingAttachment{
		AuthorName: fmt.Sprintf("%s.%s", workload, namespace),
		Text:       message,
		MrkdwnIn:   []string{"text"},
		Fields:     dfields,
	}

	dfieldString := "[ "
	for _, value := range a.Fields {
		//fmt.Println(index, "\t",value)
		dfieldString += fmt.Sprintf("{title:%s, Value:%s, Short:%s},", value.Title, value.Value, strconv.FormatBool(value.Short))
	}
	strings.TrimRight(dfieldString, ",")
	dfieldString += "]"

	text := DingDingText{
		Content: fmt.Sprintf("%s %s %s %s", a.AuthorName, a.Text, a.MrkdwnIn, dfieldString),
	}

	payload.Text = text

	err := postMessage(d.URL, payload)
	if err != nil {
		return fmt.Errorf("postMessage failed: %w", err)
	}
	return nil
}
