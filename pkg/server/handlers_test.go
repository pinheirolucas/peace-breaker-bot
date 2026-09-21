package server

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/bot"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/fsutil"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/instant"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/provider"
)

func serverAllowingHost(host string) *Server {
	return &Server{
		player:   instant.NewPlayer(),
		bot:      connectedBot(),
		registry: provider.Registry{"test": &fakeProvider{key: "test", hosts: []string{host}}},
	}
}

type fakeBot struct {
	status      bot.VoiceStatus
	identity    bot.Identity
	hasIdentity bool

	joinErr       error
	leaveErr      error
	joinedOwner   int
	joinedChannel snowflake.ID
	left          int
}

func (f *fakeBot) Status() bot.VoiceStatus { return f.status }

func (f *fakeBot) Identity() (bot.Identity, bool) { return f.identity, f.hasIdentity }

func (f *fakeBot) JoinOwner(ctx context.Context) error {
	f.joinedOwner++
	return f.joinErr
}

func (f *fakeBot) JoinChannel(ctx context.Context, channelID snowflake.ID) error {
	f.joinedChannel = channelID
	return f.joinErr
}

func (f *fakeBot) Leave() error {
	f.left++
	return f.leaveErr
}

func connectedBot() *fakeBot {
	return &fakeBot{status: bot.VoiceStatus{Connected: true}}
}

// seedCache points the shared fsutil cache at a temp dir holding a fixture for
// each link, so the handlers resolve clips without any network access.
func seedCache(t *testing.T, links ...string) {
	t.Helper()

	dir := t.TempDir()
	previous := fsutil.Default
	fsutil.Default = &fsutil.Cache{Dir: dir}
	t.Cleanup(func() { fsutil.Default = previous })

	mp3, err := os.ReadFile(filepath.Join("testdata", "clip.mp3"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	for _, link := range links {
		name := filepath.Join(dir, fmt.Sprintf("%x.mp3", md5.Sum([]byte(link))))
		if err := os.WriteFile(name, mp3, 0o644); err != nil {
			t.Fatalf("writing cache fixture: %v", err)
		}
	}
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	return out
}

func TestHandleInstantContentRejectsAnUnusableURL(t *testing.T) {
	s := New(instant.NewPlayer(), connectedBot())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instants/not-a-url/content", nil)
	req.SetPathValue("url", "not-a-url")

	rec := httptest.NewRecorder()
	s.handleInstantContent(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if got := decodeBody(t, rec)["label"]; got != "invalid_url" {
		t.Errorf("label = %v, want invalid_url", got)
	}
}

func TestHandleInstantContentReturnsTheClipAsADataURI(t *testing.T) {
	const link = "https://example.com/a.mp3"
	seedCache(t, link)

	s := serverAllowingHost("example.com")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instants/"+link+"/content", nil)
	req.SetPathValue("url", link)

	rec := httptest.NewRecorder()
	s.handleInstantContent(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	data, ok := decodeBody(t, rec)["data"].(map[string]any)
	if !ok {
		t.Fatalf("response had no data object: %s", rec.Body.String())
	}
	if data["exists"] != true {
		t.Errorf("exists = %v, want true", data["exists"])
	}
	content, _ := data["content"].(string)
	if !strings.HasPrefix(content, "data:audio/mp3;base64,") {
		t.Errorf("content = %.40q, want an mp3 data URI", content)
	}
	// The whole clip must survive, not just the part left after type sniffing.
	if len(content) < 1000 {
		t.Errorf("content is only %d chars — the clip looks truncated", len(content))
	}
}

func TestHandleInstantContentReportsAMissingClip(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(upstream.Close)

	previous := fsutil.Default
	fsutil.Default = &fsutil.Cache{Client: upstream.Client(), Dir: t.TempDir()}
	t.Cleanup(func() { fsutil.Default = previous })

	link := upstream.URL + "/does-not-exist.mp3"

	upstreamHost, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parsing upstream URL: %v", err)
	}
	s := serverAllowingHost(upstreamHost.Hostname())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instants/"+link+"/content", nil)
	req.SetPathValue("url", link)

	rec := httptest.NewRecorder()
	s.handleInstantContent(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	data, ok := decodeBody(t, rec)["data"].(map[string]any)
	if !ok {
		t.Fatalf("response had no data object: %s", rec.Body.String())
	}
	if data["exists"] != false {
		t.Errorf("exists = %v, want false", data["exists"])
	}
}

func TestHandleInstantContentReportsABadUpstreamStatus(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(upstream.Close)

	previous := fsutil.Default
	fsutil.Default = &fsutil.Cache{Client: upstream.Client(), Dir: t.TempDir()}
	t.Cleanup(func() { fsutil.Default = previous })

	link := upstream.URL + "/a.mp3"

	upstreamHost, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parsing upstream URL: %v", err)
	}
	s := serverAllowingHost(upstreamHost.Hostname())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instants/"+link+"/content", nil)
	req.SetPathValue("url", link)

	rec := httptest.NewRecorder()
	s.handleInstantContent(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	if got := decodeBody(t, rec)["label"]; got != "bad_http_status" {
		t.Errorf("label = %v, want bad_http_status", got)
	}
}

func TestHandleInstantContentRejectsNonMp3Content(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not an mp3 file"))
	}))
	t.Cleanup(upstream.Close)

	previous := fsutil.Default
	fsutil.Default = &fsutil.Cache{Client: upstream.Client(), Dir: t.TempDir()}
	t.Cleanup(func() { fsutil.Default = previous })

	link := upstream.URL + "/a.mp3"

	upstreamHost, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parsing upstream URL: %v", err)
	}
	s := serverAllowingHost(upstreamHost.Hostname())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instants/"+link+"/content", nil)
	req.SetPathValue("url", link)

	rec := httptest.NewRecorder()
	s.handleInstantContent(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	if got := decodeBody(t, rec)["label"]; got != "unsuported_audio_format" {
		t.Errorf("label = %v, want unsuported_audio_format", got)
	}
}

func TestHandleBotPlayRejectsAnInvalidBody(t *testing.T) {
	s := New(instant.NewPlayer(), connectedBot())

	rec := httptest.NewRecorder()
	s.handleBotPlay(rec, httptest.NewRequest(http.MethodPost, "/api/v1/bot/play", strings.NewReader("not json")))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if got := decodeBody(t, rec)["label"]; got != "invalid_body" {
		t.Errorf("label = %v, want invalid_body", got)
	}
}

func TestHandleBotPlayRejectsAnInvalidURL(t *testing.T) {
	s := New(instant.NewPlayer(), connectedBot())

	rec := httptest.NewRecorder()
	s.handleBotPlay(rec, httptest.NewRequest(http.MethodPost, "/api/v1/bot/play", strings.NewReader(`{"url":"not a url"}`)))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if got := decodeBody(t, rec)["label"]; got != "invalid_url" {
		t.Errorf("label = %v, want invalid_url", got)
	}
}

func TestHandleBotPlayRejectsWhenTheBotHasNoVoiceConnection(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(upstream.Close)

	previous := fsutil.Default
	fsutil.Default = &fsutil.Cache{Client: upstream.Client(), Dir: t.TempDir()}
	t.Cleanup(func() { fsutil.Default = previous })

	link := upstream.URL + "/does-not-exist.mp3"

	s := New(instant.NewPlayer(), &fakeBot{status: bot.VoiceStatus{Connected: false}})

	rec := httptest.NewRecorder()
	s.handleBotPlay(rec, httptest.NewRequest(http.MethodPost, "/api/v1/bot/play", strings.NewReader(`{"url":"`+link+`"}`)))

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
	if got := decodeBody(t, rec)["label"]; got != "bot_not_connected" {
		t.Errorf("label = %v, want bot_not_connected", got)
	}
}

func TestHandleBotPlayReportsANotFoundClipAs404(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(upstream.Close)

	previous := fsutil.Default
	fsutil.Default = &fsutil.Cache{Client: upstream.Client(), Dir: t.TempDir()}
	t.Cleanup(func() { fsutil.Default = previous })

	link := upstream.URL + "/does-not-exist.mp3"

	s := New(instant.NewPlayer(), connectedBot())

	rec := httptest.NewRecorder()
	s.handleBotPlay(rec, httptest.NewRequest(http.MethodPost, "/api/v1/bot/play", strings.NewReader(`{"url":"`+link+`"}`)))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if got := decodeBody(t, rec)["label"]; got != "instant_not_found" {
		t.Errorf("label = %v, want instant_not_found", got)
	}
}

// TestHandleBotPlayReturnsOnlyOneResponseForUnsupportedAudio guards against a
// missing return after the fsutil.ErrUnsuportedAudioFormat case: without it,
// control falls out of the switch and into writeSuccessResponse, appending a
// second JSON object onto the same body.
func TestHandleBotPlayReturnsOnlyOneResponseForUnsupportedAudio(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not an mp3 file"))
	}))
	t.Cleanup(upstream.Close)

	previous := fsutil.Default
	fsutil.Default = &fsutil.Cache{Client: upstream.Client(), Dir: t.TempDir()}
	t.Cleanup(func() { fsutil.Default = previous })

	link := upstream.URL + "/a.mp3"

	s := New(instant.NewPlayer(), connectedBot())

	rec := httptest.NewRecorder()
	s.handleBotPlay(rec, httptest.NewRequest(http.MethodPost, "/api/v1/bot/play", strings.NewReader(`{"url":"`+link+`"}`)))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}

	dec := json.NewDecoder(bytes.NewReader(rec.Body.Bytes()))
	var out map[string]any
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if out["label"] != "unsuported_audio_format" {
		t.Errorf("label = %v, want unsuported_audio_format", out["label"])
	}
	if dec.More() {
		t.Errorf("response body carries more than one JSON object: %s", rec.Body.String())
	}
}

func TestHandleBotPlayReturnsTheExitReasonWhenPlaybackEnds(t *testing.T) {
	const link = "https://example.com/b.mp3"
	seedCache(t, link)

	player := instant.NewPlayer()
	s := New(player, connectedBot())

	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		s.handleBotPlay(rec, httptest.NewRequest(http.MethodPost, "/api/v1/bot/play", strings.NewReader(`{"url":"`+link+`"}`)))
		close(done)
	}()

	// Stand in for the bot loop: take the queued path, then report completion.
	pb, _ := player.Next()
	pb.End()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleBotPlay did not return after End()")
	}

	data, ok := decodeBody(t, rec)["data"].(map[string]any)
	if !ok {
		t.Fatalf("response had no data object: %s", rec.Body.String())
	}
	if data["exitReason"] != "end" {
		t.Errorf("exitReason = %v, want end", data["exitReason"])
	}
}

func TestHandleBotStopReleasesAnInFlightPlay(t *testing.T) {
	const link = "https://example.com/c.mp3"
	seedCache(t, link)

	player := instant.NewPlayer()
	s := New(player, connectedBot())

	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		s.handleBotPlay(rec, httptest.NewRequest(http.MethodPost, "/api/v1/bot/play", strings.NewReader(`{"url":"`+link+`"}`)))
		close(done)
	}()

	pb, _ := player.Next()

	s.handleBotStop(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/bot/stop", nil))

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("POST /api/v1/bot/stop did not release the in-flight /api/v1/bot/play")
	}
	<-pb.Context().Done()

	data, _ := decodeBody(t, rec)["data"].(map[string]any)
	if data["exitReason"] != "stop" {
		t.Errorf("exitReason = %v, want stop", data["exitReason"])
	}
}

func TestHandleBotStopIsSafeWhenNothingIsPlaying(t *testing.T) {
	s := New(instant.NewPlayer(), connectedBot())

	rec := httptest.NewRecorder()
	s.handleBotStop(rec, httptest.NewRequest(http.MethodPost, "/api/v1/bot/stop", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestHandleBotStatusReflectsAConnectedBot(t *testing.T) {
	status := bot.VoiceStatus{
		Connected:   true,
		GuildID:     123,
		GuildName:   "Guild",
		ChannelID:   456,
		ChannelName: "General",
	}
	s := New(instant.NewPlayer(), &fakeBot{status: status})

	rec := httptest.NewRecorder()
	s.handleBotStatus(rec, httptest.NewRequest(http.MethodGet, "/api/v1/bot/status", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	data, ok := decodeBody(t, rec)["data"].(map[string]any)
	if !ok {
		t.Fatalf("response had no data object: %s", rec.Body.String())
	}
	if data["connected"] != true {
		t.Errorf("connected = %v, want true", data["connected"])
	}
	if data["guildId"] != "123" {
		t.Errorf("guildId = %v, want \"123\"", data["guildId"])
	}
	if data["guildName"] != "Guild" {
		t.Errorf("guildName = %v, want Guild", data["guildName"])
	}
	if data["channelId"] != "456" {
		t.Errorf("channelId = %v, want \"456\"", data["channelId"])
	}
	if data["channelName"] != "General" {
		t.Errorf("channelName = %v, want General", data["channelName"])
	}
}

func TestHandleBotStatusOmitsEmptyNamesWhenConnectedWithNoCacheHit(t *testing.T) {
	status := bot.VoiceStatus{Connected: true, GuildID: 123, ChannelID: 456}
	s := New(instant.NewPlayer(), &fakeBot{status: status})

	rec := httptest.NewRecorder()
	s.handleBotStatus(rec, httptest.NewRequest(http.MethodGet, "/api/v1/bot/status", nil))

	data, ok := decodeBody(t, rec)["data"].(map[string]any)
	if !ok {
		t.Fatalf("response had no data object: %s", rec.Body.String())
	}
	if _, present := data["guildName"]; present {
		t.Errorf("guildName present in response, want omitted: %v", data["guildName"])
	}
	if _, present := data["channelName"]; present {
		t.Errorf("channelName present in response, want omitted: %v", data["channelName"])
	}
	if data["guildId"] != "123" || data["channelId"] != "456" {
		t.Errorf("guildId/channelId = %v/%v, want 123/456", data["guildId"], data["channelId"])
	}
}

func TestHandleBotStatusReflectsADisconnectedBot(t *testing.T) {
	s := New(instant.NewPlayer(), &fakeBot{status: bot.VoiceStatus{Connected: false}})

	rec := httptest.NewRecorder()
	s.handleBotStatus(rec, httptest.NewRequest(http.MethodGet, "/api/v1/bot/status", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	data, ok := decodeBody(t, rec)["data"].(map[string]any)
	if !ok {
		t.Fatalf("response had no data object: %s", rec.Body.String())
	}
	if data["connected"] != false {
		t.Errorf("connected = %v, want false", data["connected"])
	}
	for _, key := range []string{"guildId", "guildName", "channelId", "channelName"} {
		if _, present := data[key]; present {
			t.Errorf("%s present in a disconnected response, want omitted: %v", key, data[key])
		}
	}
}

func testIdentity() bot.Identity {
	return bot.Identity{ID: 345, Username: "peace-breaker", DisplayName: "Peace Breaker", AvatarURL: "https://cdn.discordapp.com/avatars/345/a.png"}
}

func TestHandleBotStatusReportsTheBotIdentityWhileDisconnected(t *testing.T) {
	s := New(instant.NewPlayer(), &fakeBot{identity: testIdentity(), hasIdentity: true})

	rec := httptest.NewRecorder()
	s.handleBotStatus(rec, httptest.NewRequest(http.MethodGet, "/api/v1/bot/status", nil))

	data, ok := decodeBody(t, rec)["data"].(map[string]any)
	if !ok {
		t.Fatalf("response had no data object: %s", rec.Body.String())
	}
	if data["connected"] != false {
		t.Errorf("connected = %v, want false", data["connected"])
	}
	botData, ok := data["bot"].(map[string]any)
	if !ok {
		t.Fatalf("response had no bot object: %s", rec.Body.String())
	}
	want := map[string]string{
		"id":          "345",
		"username":    "peace-breaker",
		"displayName": "Peace Breaker",
		"avatarUrl":   "https://cdn.discordapp.com/avatars/345/a.png",
		"profileUrl":  "https://discord.com/users/345",
		"inviteUrl":   "https://discord.com/oauth2/authorize?client_id=345&scope=bot&permissions=3214336",
	}
	for key, value := range want {
		if botData[key] != value {
			t.Errorf("bot.%s = %v, want %q", key, botData[key], value)
		}
	}
	if _, present := data["channelUrl"]; present {
		t.Errorf("channelUrl present in a disconnected response: %v", data["channelUrl"])
	}
}

func TestHandleBotStatusOmitsTheBotBeforeTheGatewayIsReady(t *testing.T) {
	s := New(instant.NewPlayer(), &fakeBot{status: bot.VoiceStatus{Connected: false}})

	rec := httptest.NewRecorder()
	s.handleBotStatus(rec, httptest.NewRequest(http.MethodGet, "/api/v1/bot/status", nil))

	data, ok := decodeBody(t, rec)["data"].(map[string]any)
	if !ok {
		t.Fatalf("response had no data object: %s", rec.Body.String())
	}
	if _, present := data["bot"]; present {
		t.Errorf("bot present in response, want omitted: %v", data["bot"])
	}
}

func TestHandleBotStatusOmitsAnEmptyAvatarURL(t *testing.T) {
	identity := testIdentity()
	identity.AvatarURL = ""
	s := New(instant.NewPlayer(), &fakeBot{identity: identity, hasIdentity: true})

	rec := httptest.NewRecorder()
	s.handleBotStatus(rec, httptest.NewRequest(http.MethodGet, "/api/v1/bot/status", nil))

	data := decodeBody(t, rec)["data"].(map[string]any)
	botData := data["bot"].(map[string]any)
	if _, present := botData["avatarUrl"]; present {
		t.Errorf("avatarUrl present, want omitted: %v", botData["avatarUrl"])
	}
}

func TestHandleBotStatusLinksToTheConnectedChannel(t *testing.T) {
	status := bot.VoiceStatus{Connected: true, GuildID: 123, ChannelID: 456}
	s := New(instant.NewPlayer(), &fakeBot{status: status, identity: testIdentity(), hasIdentity: true})

	rec := httptest.NewRecorder()
	s.handleBotStatus(rec, httptest.NewRequest(http.MethodGet, "/api/v1/bot/status", nil))

	data := decodeBody(t, rec)["data"].(map[string]any)
	if data["channelUrl"] != "https://discord.com/channels/123/456" {
		t.Errorf("channelUrl = %v, want https://discord.com/channels/123/456", data["channelUrl"])
	}
}
