package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/httpapi"
	"cyberagent-workbench/internal/webui"
)

const apiTokenEnvironment = "CYBERAGENT_API_TOKEN"

const apiControlTokenEnvironment = "CYBERAGENT_API_CONTROL_TOKEN"

func (a *App) apiCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("API subcommand is required")
	}
	switch args[0] {
	case "serve":
		return a.apiServeCommand(ctx, args[1:])
	case "openapi":
		return a.apiOpenAPICommand(args[1:])
	default:
		return fmt.Errorf("unknown API subcommand %q", args[0])
	}
}

func (a *App) apiServeCommand(ctx context.Context, args []string) error {
	fs := newFlagSet("api serve", a.errOut)
	listenAddress := fs.String("listen", httpapi.DefaultListenAddress, "loopback listen address")
	uiDirectory := fs.String("ui-dir", "", "optional built Web UI directory")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"listen": true, "ui-dir": true})); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: cyberagent api serve [--listen <loopback-host:port>] [--ui-dir <built-web-directory>]")
	}

	accessToken := os.Getenv(apiTokenEnvironment)
	controlToken := os.Getenv(apiControlTokenEnvironment)
	generated := accessToken == ""
	if generated {
		var err error
		accessToken, err = httpapi.GenerateAccessToken()
		if err != nil {
			return err
		}
	}
	var uiBundle *webui.Bundle
	if strings.TrimSpace(*uiDirectory) != "" {
		var err error
		uiBundle, err = webui.LoadDirectory(*uiDirectory)
		if err != nil {
			return apperror.Wrap(apperror.CodeInvalidArgument, "invalid Web UI directory", err)
		}
	}
	if err := a.ensureStore(); err != nil {
		return err
	}
	api, err := httpapi.New(a.store, httpapi.Config{
		AccessToken: accessToken, ControlToken: controlToken, AppVersion: Version,
		UIHandler: uiBundle,
	})
	if err != nil {
		return err
	}
	listener, err := httpapi.ListenLoopback(ctx, *listenAddress)
	if err != nil {
		return err
	}
	server, err := httpapi.NewServer(api, log.New(a.errOut, "api: ", log.LstdFlags))
	if err != nil {
		_ = listener.Close()
		return err
	}
	origin := "http://" + listener.Addr().String()
	baseURL := origin + "/api/v1"
	fmt.Fprintf(a.out, "api_url: %s\napi_version: %s\napi_token_generated: %t\napi_control_enabled: %t\n",
		baseURL, httpapi.Version, generated, controlToken != "")
	if uiBundle != nil {
		fmt.Fprintf(a.out, "ui_url: %s/\nui_source: %s\nui_assets: %d\nui_digest: %s\n",
			origin, uiBundle.Source(), uiBundle.AssetCount(), uiBundle.Digest())
	}
	if generated {
		fmt.Fprintf(a.out, "api_token: %s\n", accessToken)
	} else {
		fmt.Fprintf(a.out, "api_token_source: %s\n", apiTokenEnvironment)
	}
	if controlToken != "" {
		fmt.Fprintf(a.out, "api_control_token_source: %s\n", apiControlTokenEnvironment)
	}
	fmt.Fprintln(a.out, "note: the API is loopback-only; control is separately authorized and tokens are not persisted")
	return server.Serve(ctx, listener)
}

func (a *App) apiOpenAPICommand(args []string) error {
	fs := newFlagSet("api openapi", a.errOut)
	output := fs.String("output", "", "optional output file")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"output": true})); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: cyberagent api openapi [--output <path>]")
	}
	document, err := httpapi.GenerateOpenAPI()
	if err != nil {
		return err
	}
	if strings.TrimSpace(*output) == "" {
		_, err = a.out.Write(document)
		return err
	}
	path := filepath.Clean(strings.TrimSpace(*output))
	if err := os.WriteFile(path, document, 0o644); err != nil {
		return fmt.Errorf("write OpenAPI document: %w", err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		absolute = path
	}
	fmt.Fprintf(a.out, "openapi_written: %s\n", absolute)
	return nil
}
