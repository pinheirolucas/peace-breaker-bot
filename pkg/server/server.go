package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/disgoorg/snowflake/v2"
	"golang.org/x/text/language"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/bot"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/fsutil"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/i18n"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/instant"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/provider"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/provider/myinstants"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/provider/soundboardguy"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/provider/soundbuttons"
)

const autodiscoveryServiceName = "_myinstants._tcp"

const defaultProviderKey = "myinstants"

var defaultRegistry = newRegistry(
	myinstants.New(),
	soundboardguy.New(),
	soundbuttons.New(),
)

func newRegistry(providers ...provider.Provider) provider.Registry {
	r := make(provider.Registry, len(providers))
	for _, p := range providers {
		r[p.Key()] = p
	}

	return r
}

// BotControl is the bot's voice connection and identity, as the server needs them.
type BotControl interface {
	Status() bot.VoiceStatus
	Identity() (bot.Identity, bool)
	JoinOwner(ctx context.Context) error
	JoinChannel(ctx context.Context, channelID snowflake.ID) error
	Leave() error
}

type Server struct {
	player *instant.Player
	bot    BotControl

	registry provider.Registry
}

func New(player *instant.Player, bot BotControl) *Server {
	return &Server{player: player, bot: bot}
}

func (s *Server) providers() provider.Registry {
	if s.registry != nil {
		return s.registry
	}

	return defaultRegistry
}

func (s *Server) providerKeys() []string {
	registry := s.providers()

	keys := make([]string, 0, len(registry))
	for key := range registry {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}

func (s *Server) Start(address string) error {
	r := http.NewServeMux()

	r.HandleFunc("POST /api/v1/bot/play", s.handleBotPlay)
	r.HandleFunc("POST /api/v1/bot/stop", s.handleBotStop)
	r.HandleFunc("POST /api/v1/bot/join", s.handleBotJoin)
	r.HandleFunc("POST /api/v1/bot/leave", s.handleBotLeave)
	r.HandleFunc("GET /api/v1/bot/status", s.handleBotStatus)
	r.HandleFunc("GET /api/v1/instants", s.handleListInstants)
	r.HandleFunc("GET /api/v1/instants/{url}/content", s.handleInstantContent)
	r.HandleFunc("GET /api/v1/providers", s.handleListProviders)
	r.HandleFunc("GET /api/v1/openapi.yaml", s.handleOpenAPISpec)
	r.HandleFunc("GET /api/docs", s.handleDocs)

	srv := &http.Server{
		Handler: loggingMiddleware(corsMiddleware(r)),
		Addr:    address,
	}

	_, port := getHostAndPortFromAddress(address)
	if port == 0 {
		return errors.New("invalid address to bind")
	}

	autodiscovery, err := newAutodiscoveryServer(autodiscoveryServiceName, port)
	if err != nil {
		return fmt.Errorf("unable to register autodiscovery server for myinstants: %w", err)
	}
	defer autodiscovery.Shutdown()

	slog.Debug("providers registered", "keys", s.providerKeys())
	slog.Info("listening for http connections", "address", address)
	slog.Info("registering autodiscovery server", "service", autodiscoveryServiceName)
	return srv.ListenAndServe()
}

type response struct {
	Label   string      `json:"label,omitempty"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func languageFor(r *http.Request) language.Tag {
	if header := r.Header.Get("Accept-Language"); header != "" {
		return i18n.MatchAcceptLanguage(header)
	}

	return i18n.Supported[0]
}

func writeErrorMessage(w http.ResponseWriter, status int, lang language.Tag, label string) {
	out := &response{
		Label:   label,
		Message: i18n.Text(lang, label),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(out); err != nil {
		http.Error(w, "unknown error", http.StatusInternalServerError)
	}
}

func writeSuccessResponse(w http.ResponseWriter, data interface{}) {
	out := &response{
		Data: data,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		http.Error(w, "unknown error", http.StatusInternalServerError)
	}
}

type botPlayRequest struct {
	URL string `json:"url,omitempty"`
}

type botPlayResponse struct {
	ExitReason string `json:"exitReason,omitempty"`
}

func (s *Server) handleBotPlay(w http.ResponseWriter, r *http.Request) {
	lang := languageFor(r)

	in := new(botPlayRequest)
	if err := json.NewDecoder(r.Body).Decode(in); err != nil {
		writeErrorMessage(w, http.StatusBadRequest, lang, "invalid_body")
		return
	}

	if !s.bot.Status().Connected {
		slog.Debug("play refused, bot not connected", "url", in.URL)
		writeErrorMessage(w, http.StatusConflict, lang, "bot_not_connected")
		return
	}

	exitReason, err := s.player.Play(in.URL)
	switch err {
	case nil:
	case instant.ErrInvalidLink:
		writeErrorMessage(w, http.StatusBadRequest, lang, "invalid_url")
		return
	case fsutil.ErrNotFound:
		writeErrorMessage(w, http.StatusNotFound, lang, "instant_not_found")
		return
	case fsutil.ErrUnsuportedAudioFormat:
		writeErrorMessage(w, http.StatusUnprocessableEntity, lang, "unsuported_audio_format")
		return
	default:
		slog.Error("play failed", "url", in.URL, "err", err)
		writeErrorMessage(w, http.StatusInternalServerError, lang, "unknown_error")
		return
	}

	writeSuccessResponse(w, &botPlayResponse{exitReason})
}

func (s *Server) handleBotStop(w http.ResponseWriter, r *http.Request) {
	s.player.Stop()
}

type botIdentityResponse struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	AvatarURL   string `json:"avatarUrl,omitempty"`
	ProfileURL  string `json:"profileUrl"`
	InviteURL   string `json:"inviteUrl"`
}

type botStatusResponse struct {
	Connected   bool                 `json:"connected"`
	Bot         *botIdentityResponse `json:"bot,omitempty"`
	GuildID     string               `json:"guildId,omitempty"`
	GuildName   string               `json:"guildName,omitempty"`
	ChannelID   string               `json:"channelId,omitempty"`
	ChannelName string               `json:"channelName,omitempty"`
	ChannelURL  string               `json:"channelUrl,omitempty"`
}

func (s *Server) botStatusResponse() *botStatusResponse {
	status := s.bot.Status()
	out := &botStatusResponse{Connected: status.Connected}
	if identity, ok := s.bot.Identity(); ok {
		out.Bot = &botIdentityResponse{
			ID:          identity.ID.String(),
			Username:    identity.Username,
			DisplayName: identity.DisplayName,
			AvatarURL:   identity.AvatarURL,
			ProfileURL:  identity.ProfileURL(),
			InviteURL:   identity.InviteURL(),
		}
	}
	if status.Connected {
		out.GuildID, out.GuildName = status.GuildID.String(), status.GuildName
		out.ChannelID, out.ChannelName = status.ChannelID.String(), status.ChannelName
		if status.GuildID != 0 && status.ChannelID != 0 {
			out.ChannelURL = bot.ChannelURL(status.GuildID, status.ChannelID)
		}
	}
	return out
}

func (s *Server) handleBotStatus(w http.ResponseWriter, r *http.Request) {
	writeSuccessResponse(w, s.botStatusResponse())
}

type botJoinRequest struct {
	ChannelID *string `json:"channelId"`
}

func (s *Server) handleBotJoin(w http.ResponseWriter, r *http.Request) {
	lang := languageFor(r)

	in := new(botJoinRequest)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(in); err != nil && !errors.Is(err, io.EOF) {
		writeErrorMessage(w, http.StatusBadRequest, lang, "invalid_body")
		return
	}

	var err error
	if in.ChannelID == nil {
		err = s.bot.JoinOwner(r.Context())
	} else {
		channelID, parseErr := snowflake.Parse(*in.ChannelID)
		if parseErr != nil || channelID == 0 {
			writeErrorMessage(w, http.StatusBadRequest, lang, "invalid_body")
			return
		}

		err = s.bot.JoinChannel(r.Context(), channelID)
	}
	if err != nil {
		writeVoiceError(w, lang, err)
		return
	}

	writeSuccessResponse(w, s.botStatusResponse())
}

func (s *Server) handleBotLeave(w http.ResponseWriter, r *http.Request) {
	if err := s.bot.Leave(); err != nil {
		writeVoiceError(w, languageFor(r), err)
		return
	}

	writeSuccessResponse(w, s.botStatusResponse())
}

func writeVoiceError(w http.ResponseWriter, lang language.Tag, err error) {
	switch {
	case errors.Is(err, bot.ErrNotReady):
		writeErrorMessage(w, http.StatusServiceUnavailable, lang, "bot_not_ready")
	case errors.Is(err, bot.ErrChannelNotFound):
		writeErrorMessage(w, http.StatusNotFound, lang, "channel_not_found")
	case errors.Is(err, bot.ErrOwnerNotInVoice):
		writeErrorMessage(w, http.StatusConflict, lang, "owner_not_in_voice")
	case errors.Is(err, bot.ErrOwnerUnknown):
		writeErrorMessage(w, http.StatusConflict, lang, "owner_unknown")
	case errors.Is(err, bot.ErrNotVoiceChannel):
		writeErrorMessage(w, http.StatusUnprocessableEntity, lang, "not_voice_channel")
	case errors.Is(err, bot.ErrJoinFailed):
		writeErrorMessage(w, http.StatusBadGateway, lang, "voice_join_failed")
	default:
		slog.Error("voice request failed", "err", err)
		writeErrorMessage(w, http.StatusInternalServerError, lang, "unknown_error")
	}
}

func (s *Server) allowedContentHosts() map[string]struct{} {
	hosts := make(map[string]struct{})
	for _, p := range s.providers() {
		for _, h := range p.AllowedContentHosts() {
			hosts[strings.ToLower(h)] = struct{}{}
		}
	}

	return hosts
}

func (s *Server) handleInstantContent(w http.ResponseWriter, r *http.Request) {
	lang := languageFor(r)

	rawURL := r.PathValue("url")
	if !instant.IsLinkValid(rawURL) {
		slog.Debug("content url rejected", "reason", "malformed", "url", rawURL)
		writeErrorMessage(w, http.StatusBadRequest, lang, "invalid_url")
		return
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		slog.Debug("content url rejected", "reason", "malformed", "url", rawURL)
		writeErrorMessage(w, http.StatusBadRequest, lang, "invalid_url")
		return
	}
	if _, ok := s.allowedContentHosts()[strings.ToLower(parsed.Hostname())]; !ok {
		slog.Debug("content url rejected", "reason", "host-not-allowed", "host", parsed.Hostname())
		writeErrorMessage(w, http.StatusBadRequest, lang, "invalid_url")
		return
	}

	info, err := instant.GetPlayable(rawURL)
	switch {
	case err == nil:
	case errors.Is(err, fsutil.ErrUnsuportedAudioFormat):
		writeErrorMessage(w, http.StatusUnprocessableEntity, lang, "unsuported_audio_format")
		return
	case errors.Is(err, fsutil.ErrUpstreamUnavailable):
		slog.Warn("instant content unavailable upstream", "url", rawURL, "err", err)
		writeErrorMessage(w, http.StatusBadGateway, lang, "bad_http_status")
		return
	default:
		slog.Error("instant content failed", "url", rawURL, "err", err)
		writeErrorMessage(w, http.StatusInternalServerError, lang, "unknown_error")
		return
	}

	writeSuccessResponse(w, info)
}

type providerInfo struct {
	Key            string `json:"key"`
	Name           string `json:"name"`
	SupportsSearch bool   `json:"supportsSearch"`
	SupportsRegion bool   `json:"supportsRegion"`
}

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	registry := s.providers()
	keys := s.providerKeys()

	out := make([]providerInfo, 0, len(keys))
	for _, key := range keys {
		p := registry[key]
		out = append(out, providerInfo{
			Key:            p.Key(),
			Name:           p.DisplayName(),
			SupportsSearch: true,
			SupportsRegion: p.SupportsRegion(),
		})
	}

	writeSuccessResponse(w, out)
}

type instantButton struct {
	Name string `json:"name,omitempty"`
	URL  string `json:"url,omitempty"`
}

type instantListResponse struct {
	Instants []*instantButton `json:"instants"`
	Pages    int              `json:"pages"`
}

func toInstantListResponse(list *provider.ListResult) *instantListResponse {
	instants := make([]*instantButton, 0, len(list.Instants))
	for _, i := range list.Instants {
		instants = append(instants, &instantButton{Name: i.Name, URL: i.URL})
	}

	return &instantListResponse{Instants: instants, Pages: list.Pages}
}

func (s *Server) handleListInstants(w http.ResponseWriter, r *http.Request) {
	lang := languageFor(r)
	vars := r.URL.Query()

	providerKey := strings.TrimSpace(vars.Get("provider"))
	if providerKey == "" {
		providerKey = defaultProviderKey
	}

	p, ok := s.providers().Get(providerKey)
	if !ok {
		slog.Debug("provider not found", "provider", providerKey)
		writeErrorMessage(w, http.StatusNotFound, lang, "provider_not_found")
		return
	}

	page, err := strconv.Atoi(strings.TrimSpace(vars.Get("page")))
	if err != nil || page < 1 {
		page = 1
	}

	params := provider.ListParams{
		Page:   page,
		Search: strings.TrimSpace(vars.Get("search")),
		Region: strings.TrimSpace(vars.Get("region")),
	}

	list, err := p.List(params)
	switch {
	case err == nil:
	case errors.Is(err, provider.ErrInvalidRegion):
		writeErrorMessage(w, http.StatusBadRequest, lang, "invalid_region")
		return
	case errors.Is(err, provider.ErrUpstreamUnavailable):
		slog.Error("provider.List", "provider", providerKey, "err", err)
		writeErrorMessage(w, http.StatusBadGateway, lang, "http_request")
		return
	case errors.Is(err, provider.ErrBadUpstreamStatus):
		slog.Error("provider.List", "provider", providerKey, "err", err)
		writeErrorMessage(w, http.StatusBadGateway, lang, "bad_http_status")
		return
	case errors.Is(err, provider.ErrUnexpectedMarkup):
		slog.Error("provider.List", "provider", providerKey, "err", err)
		writeErrorMessage(w, http.StatusInternalServerError, lang, "name_link_not_matched")
		return
	default:
		slog.Error("provider.List", "provider", providerKey, "err", err)
		writeErrorMessage(w, http.StatusInternalServerError, lang, "unknown_error")
		return
	}

	slog.Debug("instants listed",
		"provider", providerKey,
		"page", params.Page,
		"search", params.Search,
		"region", params.Region,
		"count", len(list.Instants),
		"pages", list.Pages,
	)

	writeSuccessResponse(w, toInstantListResponse(list))
}
