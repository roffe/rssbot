package webhook

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

var (
	// Debug prints debug messages
	Debug = false
)

// NewMessage creates a new webhook message
func NewMessage(hookURL string, wait bool) *Message {
	u, err := url.Parse(hookURL)
	if err != nil {
		panic(err)
	}
	u.Query().Add("wait", strconv.FormatBool(wait))
	return &Message{
		url: u.String(),
	}
}

// Message is the Discord webhook JSON params, https://discord.com/developers/docs/resources/webhook#execute-webhook
type Message struct {
	url string
	// requires one of content, file, embeds
	Content   string `json:"content,omitempty"`    // the message contents (up to 2000 characters)
	Username  string `json:"username,omitempty"`   // override the default username of the webhook
	AvatarURL string `json:"avatar_url,omitempty"` // override the default avatar of the webhook
	TTS       bool   `json:"tts,omitempty"`        // true if this is a TTS message
	//File      []byte   `json:"file,omitempty"`       // the contents of the file being sent
	Embeds          []*Embed         `json:"embeds,omitempty"`           // embedded rich content
	AllowedMentions *AllowedMentions `json:"allowed_mentions,omitempty"` // allowed mentions for the message
}

// Response from Discord api
type Response struct {
	Global     bool   `json:"global"`
	Message    string `json:"message"`
	RetryAfter int    `json:"retry_after"`
}

// Send the webhook meesage
func (w *Message) Send() error {
	if err := w.IsValid(); err != nil {
		return err
	}
	b, err := json.Marshal(w)
	if err != nil {
		return err
	}

	if Debug {
		log.Println(string(b[:]))
	}

	r := bytes.NewReader(b)
	resp, err := http.Post(w.url, "application/json", r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var dresp Response
	if err := json.NewDecoder(resp.Body).Decode(&dresp); err != nil {
		if err != io.EOF {
			log.Println(err)
		}
	}

	if Debug {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		log.Println(string(body[:]))
	}
	return nil
}

// IsValid validates so the message is correct
func (w *Message) IsValid() error {
	// one of content, file, embeds
	f := 0
	if w.Content != "" {
		f++
	}
	if w.Embeds != nil {
		f++
	}

	if f > 1 {
		return errors.New("only one of content, file or embeds is allowed")
	}

	return nil
}

const (
	// AllowedMentionsRoles Controls role mentions
	AllowedMentionsRoles = "roles"
	// AllowedMentionsUsers Controls user mentions
	AllowedMentionsUsers = "users"
	// AllowedMentionsEveryone Controls @everyone and @here mentions
	AllowedMentionsEveryone = "everyone"
)

// AllowedMentions The allowed mention field allows for more granular control over mentions without various hacks to the message content.
// This will always validate against message content to avoid phantom pings (e.g. to ping everyone, you must still have @everyone in the message content),
// and check against user/bot permissions.
type AllowedMentions struct {
	Parse []string `json:"parse"`
	Roles []string `json:"roles,omitempty"`
	Users []string `json:"users,omitempty"`
}

const (
	// TypeRich rich generic embed rendered from embed attributes
	TypeRich = "rich"
	// TypeImage image embed
	TypeImage = "image"
	// TypeVideo video embed
	TypeVideo = "video"
	// TypeGifv animated gif image embed rendered as a video embed
	TypeGifv = "gifv"
	// TypeArticle article embed
	TypeArticle = "article"
	// TypeLink link embed
	TypeLink = "link"
)

// Embed is a Discord embed, https://discord.com/developers/docs/resources/channel#embed-object
type Embed struct {
	Title       string          `json:"title,omitempty"`       // title of embed
	Type        string          `json:"type,omitempty"`        // type of embed (always "rich" for webhook embeds)
	Description string          `json:"description,omitempty"` // description of embed
	URL         string          `json:"url,omitempty"`         // url of embed
	Timestamp   *time.Time      `json:"timestamp,omitempty"`   // ISO8601 timestamp	timestamp of embed content
	Color       int             `json:"color,omitempty"`       // color code of the embed
	Footer      *EmbedFooter    `json:"footer,omitempty"`      // footer information
	Image       *EmbedImage     `json:"image,omitempty"`       // image information
	Thumbnail   *EmbedThumbnail `json:"thumbnail,omitempty"`   // thumbnail information
	Video       *EmbedVideo     `json:"video,omitempty"`       // video information
	Provider    *EmbedProvider  `json:"provider,omitempty"`    // provider information
	Author      *EmbedAuthor    `json:"author,omitempty"`      // author information
	Fields      []*EmbedField   `json:"fields,omitempty"`      // fields information
}

// AddEmbed adds a embedment to the message
func (w *Message) AddEmbed(e *Embed) *Message {
	if w.Embeds == nil {
		w.Embeds = []*Embed{}
	}
	w.Embeds = append(w.Embeds, e)
	return w
}

// EmbedFooter structure
type EmbedFooter struct {
	Text         string `json:"text,omitempty"`           // footer text
	IconURL      string `json:"icon_url,omitempty"`       // url of footer icon (only supports http(s) and attachments)
	ProxyIconURL string `json:"proxy_icon_url,omitempty"` // a proxied url of footer icon
}

// SetFooter for a embed
func (e *Embed) SetFooter(text, iconURL, proxyIconURL string) *Embed {
	e.Footer = &EmbedFooter{
		Text:         text,
		IconURL:      iconURL,
		ProxyIconURL: proxyIconURL,
	}
	return e
}

// EmbedImage structure
type EmbedImage struct {
	URL      string `json:"url,omitempty"`       // source url of image (only supports http(s) and attachments)
	ProxyURL string `json:"proxy_url,omitempty"` // a proxied url of the image
	Height   int    `json:"height,omitempty"`    // height of image
	Width    int    `json:"width,omitempty"`     // width of image
}

// SetImage for a embed
func (e *Embed) SetImage(url, proxyURL string, width, height int) *Embed {
	e.Image = &EmbedImage{
		URL:      url,
		ProxyURL: proxyURL,
		Height:   height,
		Width:    width,
	}
	return e
}

// EmbedThumbnail structure
type EmbedThumbnail struct {
	URL      string `json:"url,omitempty"`       // source url of thumbnail (only supports http(s) and attachments)
	ProxyURL string `json:"proxy_url,omitempty"` // a proxied url of the thumbnail
	Height   int    `json:"height,omitempty"`    // height of thumbnail
	Width    int    `json:"width,omitempty"`     // width of thumbnail
}

// SetThumbnail for a embed
func (e *Embed) SetThumbnail(url, proxyURL string, height, width int) *Embed {
	e.Thumbnail = &EmbedThumbnail{
		URL:      url,
		ProxyURL: proxyURL,
		Height:   height,
		Width:    width,
	}
	return e
}

// EmbedVideo structure
type EmbedVideo struct {
	URL    string `json:"url,omitempty"`    // source url of video
	Height int    `json:"height,omitempty"` // height of video
	Width  int    `json:"width,omitempty"`  // width of video
}

// SetVideo for a embed
func (e *Embed) SetVideo(url string, height, width int) *Embed {
	e.Video = &EmbedVideo{
		URL:    url,
		Height: height,
		Width:  width,
	}
	return e
}

// EmbedProvider structure
type EmbedProvider struct {
	Name string `json:"name,omitempty"` // name of provider
	URL  string `json:"url,omitempty"`  // url of provider
}

// SetProvider for a embed
func (e *Embed) SetProvider(name, url string) *Embed {
	e.Provider = &EmbedProvider{
		Name: name,
		URL:  url,
	}
	return e
}

// EmbedAuthor structure
type EmbedAuthor struct {
	Name         string `json:"name,omitempty"`           // name of author
	URL          string `json:"url,omitempty"`            // url of author
	IconURL      string `json:"icon_url,omitempty"`       // url of author icon (only supports http(s) and attachments)
	ProxyIconURL string `json:"proxy_icon_url,omitempty"` // a proxied url of author icon
}

// SetAuthor for a embed
func (e *Embed) SetAuthor(name, url, iconURL, proxyIconURL string) *Embed {
	e.Author = &EmbedAuthor{
		Name:         name,
		URL:          url,
		IconURL:      iconURL,
		ProxyIconURL: proxyIconURL,
	}
	return e
}

// EmbedField structure
type EmbedField struct {
	Name   string `json:"name,omitempty"`   // name of the field
	Value  string `json:"value,omitempty"`  //	value of the field
	Inline bool   `json:"inline,omitempty"` //	whether or not this field should display inline
}

// AddField to a embed
func (e *Embed) AddField(f *EmbedField) *Embed {
	if e.Fields == nil {
		e.Fields = []*EmbedField{}
	}
	e.Fields = append(e.Fields, f)
	return e
}
