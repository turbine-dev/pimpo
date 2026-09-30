package telegram

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// A Mini App is a web page Telegram opens inside the chat. Telegram hands
// the page initData, a signed list of who opened it; the page passes it
// on and only the server, which holds the bot token, can tell whether
// Telegram signed it.

// WebAppUser is the person who opened a Mini App, as Telegram signed it.
type WebAppUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username,omitempty"`
	Language  string `json:"language_code,omitempty"`
}

// WebApp is the page an inline or menu button opens.
type WebApp struct {
	URL string `json:"url"`
}

var (
	ErrInitData = errors.New("telegram sign-in data is not valid")
	ErrStale    = errors.New("telegram sign-in data is too old; open the app again")
)

// VerifyInitData checks initData exactly as Telegram describes: the
// fields but hash, sorted and joined with newlines, signed with HMAC-SHA256
// under the key HMAC-SHA256("WebAppData", bot token). It is refused when
// signed more than maxAge before now. Nothing in it is read before the
// signature checks out.
func VerifyInitData(initData, botToken string, maxAge time.Duration, now time.Time) (WebAppUser, error) {
	if botToken == "" || initData == "" || len(initData) > 8<<10 {
		return WebAppUser{}, ErrInitData
	}
	values, err := url.ParseQuery(initData)
	if err != nil {
		return WebAppUser{}, ErrInitData
	}
	var got []byte
	pairs := make([]string, 0, len(values))
	for k, vs := range values {
		// A field given twice could be read one way here and another way
		// later; Telegram never sends one twice.
		if len(vs) != 1 {
			return WebAppUser{}, ErrInitData
		}
		if k == "hash" {
			if got, err = hex.DecodeString(vs[0]); err != nil {
				return WebAppUser{}, ErrInitData
			}
			continue
		}
		pairs = append(pairs, k+"="+vs[0])
	}
	if len(got) != sha256.Size {
		return WebAppUser{}, ErrInitData
	}
	sort.Strings(pairs)
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(botToken))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(pairs, "\n")))
	if !hmac.Equal(mac.Sum(nil), got) {
		return WebAppUser{}, ErrInitData
	}
	signed, err := strconv.ParseInt(values.Get("auth_date"), 10, 64)
	if err != nil {
		return WebAppUser{}, ErrInitData
	}
	at := time.Unix(signed, 0)
	if now.Sub(at) > maxAge || at.Sub(now) > time.Minute {
		return WebAppUser{}, ErrStale
	}
	var u WebAppUser
	if err := json.Unmarshal([]byte(values.Get("user")), &u); err != nil || u.ID == 0 {
		return WebAppUser{}, ErrInitData
	}
	return u, nil
}

// SignInitData makes initData as Telegram would, for tests.
func SignInitData(botToken string, fields url.Values) string {
	pairs := make([]string, 0, len(fields))
	for k := range fields {
		pairs = append(pairs, k+"="+fields.Get(k))
	}
	sort.Strings(pairs)
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(botToken))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(pairs, "\n")))
	out := url.Values{}
	for k := range fields {
		out.Set(k, fields.Get(k))
	}
	out.Set("hash", hex.EncodeToString(mac.Sum(nil)))
	return out.Encode()
}

// SetMenuButton puts a button that opens url next to the message box of
// one chat; an empty url puts Telegram's usual menu back.
func (b Bot) SetMenuButton(ctx context.Context, chat int64, text, url string) error {
	button := map[string]any{"type": "default"}
	if url != "" {
		button = map[string]any{"type": "web_app", "text": text, "web_app": WebApp{URL: url}}
	}
	return b.call(ctx, "setChatMenuButton", map[string]any{"chat_id": chat, "menu_button": button}, nil)
}
