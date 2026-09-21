package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/bot"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/instant"
)

func postJoin(s *Server, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.handleBotJoin(rec, httptest.NewRequest(http.MethodPost, "/api/v1/bot/join", strings.NewReader(body)))
	return rec
}

func TestHandleBotJoinWithoutABodyFollowsTheOwner(t *testing.T) {
	for _, body := range []string{"", "  \n", "{}", `{"channelId": null}`} {
		fake := &fakeBot{status: bot.VoiceStatus{Connected: true, GuildID: 1, ChannelID: 2, ChannelName: "General"}}
		s := New(instant.NewPlayer(), fake)

		rec := postJoin(s, body)

		if rec.Code != http.StatusOK {
			t.Fatalf("body %q: status = %d, want %d: %s", body, rec.Code, http.StatusOK, rec.Body.String())
		}
		if fake.joinedOwner != 1 || fake.joinedChannel != 0 {
			t.Errorf("body %q: joinedOwner/joinedChannel = %d/%d, want 1/0", body, fake.joinedOwner, fake.joinedChannel)
		}
		data, ok := decodeBody(t, rec)["data"].(map[string]any)
		if !ok {
			t.Fatalf("body %q: response had no data object: %s", body, rec.Body.String())
		}
		if data["connected"] != true || data["channelId"] != "2" || data["channelName"] != "General" {
			t.Errorf("body %q: data = %v, want the connected status", body, data)
		}
	}
}

func TestHandleBotJoinWithAChannelIDJoinsThatChannel(t *testing.T) {
	fake := connectedBot()
	s := New(instant.NewPlayer(), fake)

	rec := postJoin(s, `{"channelId": "234567890123456789"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if fake.joinedChannel != 234567890123456789 || fake.joinedOwner != 0 {
		t.Errorf("joinedChannel/joinedOwner = %d/%d, want 234567890123456789/0", fake.joinedChannel, fake.joinedOwner)
	}
}

func TestHandleBotJoinRejectsABadBodyWithoutTouchingTheBot(t *testing.T) {
	bodies := map[string]string{
		"not json":            `channel`,
		"truncated json":      `{"channelId": "12`,
		"unknown field":       `{"channel": "#instants"}`,
		"name in channelId":   `{"channelId": "#instants"}`,
		"blank channelId":     `{"channelId": ""}`,
		"zero channelId":      `{"channelId": "0"}`,
		"numeric channelId":   `{"channelId": 234567890123456789}`,
		"negative channelId":  `{"channelId": "-5"}`,
		"channelId and guild": `{"channelId": "1", "guildId": "2"}`,
	}

	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			fake := connectedBot()
			s := New(instant.NewPlayer(), fake)

			rec := postJoin(s, body)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
			if got := decodeBody(t, rec)["label"]; got != "invalid_body" {
				t.Errorf("label = %v, want invalid_body", got)
			}
			if fake.joinedOwner != 0 || fake.joinedChannel != 0 {
				t.Errorf("bot was asked to join (%d/%d) despite the bad body", fake.joinedOwner, fake.joinedChannel)
			}
		})
	}
}

func TestVoiceErrorsMapToTheirStatusAndLabel(t *testing.T) {
	tests := []struct {
		err    error
		status int
		label  string
	}{
		{bot.ErrNotReady, http.StatusServiceUnavailable, "bot_not_ready"},
		{bot.ErrChannelNotFound, http.StatusNotFound, "channel_not_found"},
		{bot.ErrOwnerNotInVoice, http.StatusConflict, "owner_not_in_voice"},
		{bot.ErrOwnerUnknown, http.StatusConflict, "owner_unknown"},
		{bot.ErrNotVoiceChannel, http.StatusUnprocessableEntity, "not_voice_channel"},
		{fmt.Errorf("%w: handshake timed out", bot.ErrJoinFailed), http.StatusBadGateway, "voice_join_failed"},
		{fmt.Errorf("something else"), http.StatusInternalServerError, "unknown_error"},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			s := New(instant.NewPlayer(), &fakeBot{joinErr: tt.err, leaveErr: tt.err})

			for name, rec := range map[string]*httptest.ResponseRecorder{
				"join owner":   postJoin(s, ""),
				"join channel": postJoin(s, `{"channelId": "5"}`),
			} {
				if rec.Code != tt.status {
					t.Errorf("%s: status = %d, want %d", name, rec.Code, tt.status)
				}
				if got := decodeBody(t, rec)["label"]; got != tt.label {
					t.Errorf("%s: label = %v, want %s", name, got, tt.label)
				}
			}
		})
	}
}

func TestHandleBotJoinTranslatesErrorsPerAcceptLanguage(t *testing.T) {
	s := New(instant.NewPlayer(), &fakeBot{joinErr: bot.ErrChannelNotFound})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/bot/join", strings.NewReader(`{"channelId": "5"}`))
	req.Header.Set("Accept-Language", "pt-BR")
	s.handleBotJoin(rec, req)

	if got := decodeBody(t, rec)["message"]; got != "Esse canal de voz não foi encontrado" {
		t.Errorf("message = %v, want the pt-BR text", got)
	}
}

func TestHandleBotLeaveReportsTheDisconnectedStatus(t *testing.T) {
	fake := &fakeBot{}
	s := New(instant.NewPlayer(), fake)

	rec := httptest.NewRecorder()
	s.handleBotLeave(rec, httptest.NewRequest(http.MethodPost, "/api/v1/bot/leave", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if fake.left != 1 {
		t.Errorf("left = %d, want 1", fake.left)
	}
	data, ok := decodeBody(t, rec)["data"].(map[string]any)
	if !ok {
		t.Fatalf("response had no data object: %s", rec.Body.String())
	}
	if data["connected"] != false {
		t.Errorf("connected = %v, want false", data["connected"])
	}
}

func TestHandleBotLeaveReportsAnUnreadyBotAs503(t *testing.T) {
	s := New(instant.NewPlayer(), &fakeBot{leaveErr: bot.ErrNotReady})

	rec := httptest.NewRecorder()
	s.handleBotLeave(rec, httptest.NewRequest(http.MethodPost, "/api/v1/bot/leave", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if got := decodeBody(t, rec)["label"]; got != "bot_not_ready" {
		t.Errorf("label = %v, want bot_not_ready", got)
	}
}
