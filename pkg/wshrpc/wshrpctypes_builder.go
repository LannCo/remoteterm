// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// builder-related types and methods for wsh rpc calls
package wshrpc

import (
	"context"
)

type WshRpcBuilderInterface interface {
	ListAllAppsCommand(ctx context.Context) ([]AppInfo, error)
	ListAllEditableAppsCommand(ctx context.Context) ([]AppInfo, error)
	ListAllAppFilesCommand(ctx context.Context, data CommandListAllAppFilesData) (*CommandListAllAppFilesRtnData, error)
	ReadAppFileCommand(ctx context.Context, data CommandReadAppFileData) (*CommandReadAppFileRtnData, error)
	WriteAppFileCommand(ctx context.Context, data CommandWriteAppFileData) error
	WriteAppGoFileCommand(ctx context.Context, data CommandWriteAppGoFileData) (*CommandWriteAppGoFileRtnData, error)
	SeedBuilderAppCommand(ctx context.Context, data CommandSeedBuilderAppData) (*CommandSeedBuilderAppRtnData, error)
	DeleteAppFileCommand(ctx context.Context, data CommandDeleteAppFileData) error
	RenameAppFileCommand(ctx context.Context, data CommandRenameAppFileData) error
	WriteAppSecretBindingsCommand(ctx context.Context, data CommandWriteAppSecretBindingsData) error
	DeleteBuilderCommand(ctx context.Context, builderId string) error
	StartBuilderCommand(ctx context.Context, data CommandStartBuilderData) error
	RequestBuilderRebuildCommand(ctx context.Context, data CommandRequestBuilderRebuildData) error
	WatchBuilderAppCommand(ctx context.Context, data CommandWatchBuilderAppData) (*BuilderWatchStatusData, error)
	OpenBuilderTerminalCommand(ctx context.Context, data CommandOpenBuilderTerminalData) error
	GetBuilderAppDirCommand(ctx context.Context, data CommandGetBuilderAppDirData) (string, error)
	StopBuilderCommand(ctx context.Context, builderId string) error
	GetBuilderStatusCommand(ctx context.Context, builderId string) (*BuilderStatusData, error)
	GetBuilderOutputCommand(ctx context.Context, builderId string) ([]string, error)
	CheckGoVersionCommand(ctx context.Context) (*CommandCheckGoVersionRtnData, error)
	PublishAppCommand(ctx context.Context, data CommandPublishAppData) (*CommandPublishAppRtnData, error)
	MakeDraftFromLocalCommand(ctx context.Context, data CommandMakeDraftFromLocalData) (*CommandMakeDraftFromLocalRtnData, error)
}

type AppInfo struct {
	AppId    string       `json:"appid"`
	ModTime  int64        `json:"modtime"`
	Manifest *AppManifest `json:"manifest,omitempty"`
}

type CommandListAllAppFilesData struct {
	AppId string `json:"appid"`
}

type CommandListAllAppFilesRtnData struct {
	Path         string        `json:"path"`
	AbsolutePath string        `json:"absolutepath"`
	ParentDir    string        `json:"parentdir,omitempty"`
	Entries      []DirEntryOut `json:"entries"`
	EntryCount   int           `json:"entrycount"`
	TotalEntries int           `json:"totalentries"`
	Truncated    bool          `json:"truncated,omitempty"`
}

type DirEntryOut struct {
	Name         string `json:"name"`
	Dir          bool   `json:"dir,omitempty"`
	Symlink      bool   `json:"symlink,omitempty"`
	Size         int64  `json:"size,omitempty"`
	Mode         string `json:"mode"`
	Modified     string `json:"modified"`
	ModifiedTime string `json:"modifiedtime"`
}

type CommandReadAppFileData struct {
	AppId    string `json:"appid"`
	FileName string `json:"filename"`
}

type CommandReadAppFileRtnData struct {
	Data64   string `json:"data64"`
	NotFound bool   `json:"notfound,omitempty"`
	ModTs    int64  `json:"modts,omitempty"`
}

type CommandWriteAppFileData struct {
	AppId    string `json:"appid"`
	FileName string `json:"filename"`
	Data64   string `json:"data64"`
}

type CommandWriteAppGoFileData struct {
	AppId     string `json:"appid"`
	Data64    string `json:"data64"`
	BuilderId string `json:"builderid,omitempty"`
}

type CommandWriteAppGoFileRtnData struct {
	Data64 string `json:"data64"`
}

type CommandDeleteAppFileData struct {
	AppId    string `json:"appid"`
	FileName string `json:"filename"`
}

type CommandRenameAppFileData struct {
	AppId        string `json:"appid"`
	FromFileName string `json:"fromfilename"`
	ToFileName   string `json:"tofilename"`
}

type CommandWriteAppSecretBindingsData struct {
	AppId    string            `json:"appid"`
	Bindings map[string]string `json:"bindings"`
}

type CommandStartBuilderData struct {
	BuilderId string `json:"builderid"`
}

type CommandRequestBuilderRebuildData struct {
	BuilderId string `json:"builderid"`
}

type CommandWatchBuilderAppData struct {
	BuilderId string `json:"builderid"`
}

type BuilderWatchStatusData struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type AppMeta struct {
	Title     string `json:"title"`
	ShortDesc string `json:"shortdesc"`
	Icon      string `json:"icon"`
	IconColor string `json:"iconcolor"`
}

type SecretMeta struct {
	Desc     string `json:"desc"`
	Optional bool   `json:"optional"`
}

type AppManifest struct {
	AppMeta      AppMeta               `json:"appmeta"`
	ConfigSchema map[string]any        `json:"configschema"`
	DataSchema   map[string]any        `json:"dataschema"`
	Secrets      map[string]SecretMeta `json:"secrets"`
}

type BuilderStatusData struct {
	Status                 string            `json:"status"`
	Port                   int               `json:"port,omitempty"`
	ExitCode               int               `json:"exitcode,omitempty"`
	ErrorMsg               string            `json:"errormsg,omitempty"`
	Version                int               `json:"version"`
	Manifest               *AppManifest      `json:"manifest,omitempty"`
	SecretBindings         map[string]string `json:"secretbindings,omitempty"`
	SecretBindingsComplete bool              `json:"secretbindingscomplete"`
}

type CommandCheckGoVersionRtnData struct {
	GoStatus    string `json:"gostatus"`
	GoPath      string `json:"gopath"`
	GoVersion   string `json:"goversion"`
	ErrorString string `json:"errorstring,omitempty"`
}

type CommandPublishAppData struct {
	AppId string `json:"appid"`
}

type CommandPublishAppRtnData struct {
	PublishedAppId string `json:"publishedappid"`
}

type CommandMakeDraftFromLocalData struct {
	LocalAppId string `json:"localappid"`
}

type CommandMakeDraftFromLocalRtnData struct {
	DraftAppId string `json:"draftappid"`
}

type CommandSeedBuilderAppData struct {
	AppId string `json:"appid"`
}

type CommandSeedBuilderAppRtnData struct {
	Files []string `json:"files"`
}

type CommandOpenBuilderTerminalData struct {
	BuilderId string `json:"builderid"`
	TabId     string `json:"tabid"`
}

type CommandGetBuilderAppDirData struct {
	BuilderId string `json:"builderid"`
}
