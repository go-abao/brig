package endpoints

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/sahib/brig/catfs"
	"github.com/sahib/config"
)

type LsHandler struct {
	cfg *config.Config
	fs  *catfs.FS
}

func NewLsHandler(cfg *config.Config, fs *catfs.FS) *LsHandler {
	return &LsHandler{
		cfg: cfg,
		fs:  fs,
	}
}

type LsRequest struct {
	Root     string `json:"root"`
	MaxDepth int    `json:"max_depth"`
}

type StatInfo struct {
	Path       string `json:"path"`
	User       string `json:"user"`
	Size       uint64 `json:"size"`
	Inode      uint64 `json:"inode"`
	Depth      int    `json:"depth"`
	ModTime    int64  `json:"last_modified_ms"`
	IsDir      bool   `json:"is_dir"`
	IsPinned   bool   `json:"is_pinned"`
	IsExplicit bool   `json:"is_explicit"`
}

func toExternalStatInfo(i *catfs.StatInfo) *StatInfo {
	return &StatInfo{
		Path:       i.Path,
		User:       i.User,
		Size:       i.Size,
		Inode:      i.Inode,
		Depth:      i.Depth,
		ModTime:    i.ModTime.Unix() * 1000,
		IsDir:      i.IsDir,
		IsPinned:   i.IsPinned,
		IsExplicit: i.IsExplicit,
	}
}

type LsResponse struct {
	Success bool        `json:"success"`
	Files   []*StatInfo `json:"files"`
}

func (lh *LsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	lsReq := &LsRequest{}
	if err := json.NewDecoder(r.Body).Decode(&lsReq); err != nil {
		jsonifyErrf(w, http.StatusBadRequest, "bad json")
		return
	}

	if !validateUserForPath(lh.cfg, lsReq.Root, r) {
		http.Error(w, "not authorized", http.StatusUnauthorized)
		return
	}

	items, err := lh.fs.List(lsReq.Root, lsReq.MaxDepth)
	if err != nil {
		jsonifyErrf(w, http.StatusBadRequest, "failed to list: %v", err)
		return
	}

	files := []*StatInfo{}
	for _, item := range items {
		files = append(files, toExternalStatInfo(item))
	}

	// Sort dirs before files and sort each part alphabetically
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}

		return strings.ToLower(files[i].Path) < strings.ToLower(files[j].Path)
	})

	jsonify(w, http.StatusOK, &LsResponse{
		Success: true,
		Files:   files,
	})
}
