package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

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

// BotStatus is the bot's voice-connection state, as the server needs it.
type BotStatus interface {
	Status() bot.VoiceStatus
}

type Server struct {
	player *instant.Player
	bot    BotStatus

	registry provider.Registry
}

func New(player *instant.Player, bot BotStatus) *Server {
	return &Server{player: player, bot: bot}
}

func (s *Server) providers() provider.Registry {
	if s.registry != nil {
		return s.registry
	}

	return defaultRegistry
}

func (s *Server) Start(address string) error {
	r := http.NewServeMux()

	r.HandleFunc("POST /api/v1/bot/play", s.handleBotPlay)
	r.HandleFunc("POST /api/v1/bot/stop", s.handleBotStop)
	r.HandleFunc("GET /api/v1/bot/status", s.handleBotStatus)
	r.HandleFunc("GET /api/v1/instants", s.handleListInstants)
	r.HandleFunc("GET /api/v1/instants/{url}/content", s.handleInstantContent)
	r.HandleFunc("GET /api/v1/providers", s.handleListProviders)
	r.HandleFunc("GET /api/v1/openapi.yaml", s.handleOpenAPISpec)
	r.HandleFunc("GET /api/docs", s.handleDocs)

	srv := &http.Server{
		Handler: corsMiddleware(r),
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
		writeErrorMessage(w, http.StatusInternalServerError, lang, "unknown_error")
		return
	}

	writeSuccessResponse(w, &botPlayResponse{exitReason})
}

func (s *Server) handleBotStop(w http.ResponseWriter, r *http.Request) {
	s.player.Stop()
}

type botStatusResponse struct {
	Connected   bool   `json:"connected"`
	GuildID     string `json:"guildId,omitempty"`
	GuildName   string `json:"guildName,omitempty"`
	ChannelID   string `json:"channelId,omitempty"`
	ChannelName string `json:"channelName,omitempty"`
}

func (s *Server) handleBotStatus(w http.ResponseWriter, r *http.Request) {
	status := s.bot.Status()
	out := &botStatusResponse{Connected: status.Connected}
	if status.Connected {
		out.GuildID, out.GuildName = status.GuildID.String(), status.GuildName
		out.ChannelID, out.ChannelName = status.ChannelID.String(), status.ChannelName
	}
	writeSuccessResponse(w, out)
}

// allowedContentHosts unions every registered provider's
// AllowedContentHosts, so handleInstantContent can validate a URL against
// every provider at once rather than only the one it was originally listed
// under.
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
		writeErrorMessage(w, http.StatusBadRequest, lang, "invalid_url")
		return
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		writeErrorMessage(w, http.StatusBadRequest, lang, "invalid_url")
		return
	}
	if _, ok := s.allowedContentHosts()[strings.ToLower(parsed.Hostname())]; !ok {
		writeErrorMessage(w, http.StatusBadRequest, lang, "invalid_url")
		return
	}

	info, err := instant.GetPlayable(rawURL)
	if err != nil {
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

	keys := make([]string, 0, len(registry))
	for key := range registry {
		keys = append(keys, key)
	}
	sort.Strings(keys)

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
		writeErrorMessage(w, http.StatusNotFound, lang, "provider_not_found")
		return
	}

	page, err := strconv.Atoi(strings.TrimSpace(vars.Get("page")))
	if err != nil || page < 1 {
		page = 1
	}

	list, err := p.List(provider.ListParams{
		Page:   page,
		Search: strings.TrimSpace(vars.Get("search")),
		Region: strings.TrimSpace(vars.Get("region")),
	})
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
		writeErrorMessage(w, http.StatusInternalServerError, lang, "name_link_not_matched")
		return
	default:
		slog.Error("provider.List", "provider", providerKey, "err", err)
		writeErrorMessage(w, http.StatusInternalServerError, lang, "unknown_error")
		return
	}

	writeSuccessResponse(w, toInstantListResponse(list))
}
