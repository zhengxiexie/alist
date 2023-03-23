package handles

import (
	"fmt"
	"io"
	stdpath "path"
	"regexp"

	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/sign"
	"github.com/alist-org/alist/v3/pkg/generic"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type MkdirOrLinkReq struct {
	Path string `json:"path" form:"path"`
}

func FsMkdir(c *gin.Context) {
	var req MkdirOrLinkReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	reqPath, err := user.JoinPath(req.Path)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	if !user.CanWrite() {
		meta, err := op.GetNearestMeta(stdpath.Dir(reqPath))
		if err != nil {
			if !errors.Is(errors.Cause(err), errs.MetaNotFound) {
				common.ErrorResp(c, err, 500, true)
				return
			}
		}
		if !common.CanWrite(meta, reqPath) {
			common.ErrorResp(c, errs.PermissionDenied, 403)
			return
		}
	}
	if err := fs.MakeDir(c, reqPath); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c)
}

type MoveCopyReq struct {
	SrcDir string   `json:"src_dir"`
	DstDir string   `json:"dst_dir"`
	Names  []string `json:"names"`
}

func FsMove(c *gin.Context) {
	var req MoveCopyReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if len(req.Names) == 0 {
		common.ErrorStrResp(c, "Empty file names", 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	if !user.CanMove() {
		common.ErrorResp(c, errs.PermissionDenied, 403)
		return
	}
	srcDir, err := user.JoinPath(req.SrcDir)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	dstDir, err := user.JoinPath(req.DstDir)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	for i, name := range req.Names {
		err := fs.Move(c, stdpath.Join(srcDir, name), dstDir, len(req.Names) > i+1)
		if err != nil {
			common.ErrorResp(c, err, 500)
			return
		}
	}
	common.SuccessResp(c)
}

type RecursiveMoveReq struct {
	SrcDir string `json:"src_dir"`
	DstDir string `json:"dst_dir"`
}

func FsRecursiveMove(c *gin.Context) {
	var req RecursiveMoveReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}

	user := c.MustGet("user").(*model.User)
	if !user.CanMove() {
		common.ErrorResp(c, errs.PermissionDenied, 403)
		return
	}
	srcDir, err := user.JoinPath(req.SrcDir)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	dstDir, err := user.JoinPath(req.DstDir)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}

	meta, err := op.GetNearestMeta(srcDir)
	if err != nil {
		if !errors.Is(errors.Cause(err), errs.MetaNotFound) {
			common.ErrorResp(c, err, 500, true)
			return
		}
	}
	c.Set("meta", meta)

	rootFiles, err := fs.List(c, srcDir, false)
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}

	// record the file path
	filePathMap := make(map[model.Obj]string)
	movingFiles := generic.NewQueue[model.Obj]()
	for _, file := range rootFiles {
		movingFiles.Push(file)
		filePathMap[file] = srcDir
	}

	for !movingFiles.IsEmpty() {

		movingFile := movingFiles.Pop()
		movingFilePath := fmt.Sprintf("%s/%s", filePathMap[movingFile], movingFile.GetName())
		if movingFile.IsDir() {
			// directory, recursive move
			subFilePath := movingFilePath
			subFiles, err := fs.List(c, subFilePath, true)
			if err != nil {
				common.ErrorResp(c, err, 500)
				return
			}
			for _, subFile := range subFiles {
				movingFiles.Push(subFile)
				filePathMap[subFile] = subFilePath
			}
		} else {

			if movingFilePath == dstDir {
				// same directory, don't move
				continue
			}

			// move
			err := fs.Move(c, movingFilePath, dstDir, movingFiles.IsEmpty())
			if err != nil {
				common.ErrorResp(c, err, 500)
				return
			}
		}

	}

	common.SuccessResp(c)
}

func FsCopy(c *gin.Context) {
	var req MoveCopyReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if len(req.Names) == 0 {
		common.ErrorStrResp(c, "Empty file names", 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	if !user.CanCopy() {
		common.ErrorResp(c, errs.PermissionDenied, 403)
		return
	}
	srcDir, err := user.JoinPath(req.SrcDir)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	dstDir, err := user.JoinPath(req.DstDir)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	var addedTask []string
	for i, name := range req.Names {
		ok, err := fs.Copy(c, stdpath.Join(srcDir, name), dstDir, len(req.Names) > i+1)
		if ok {
			addedTask = append(addedTask, name)
		}
		if err != nil {
			common.ErrorResp(c, err, 500)
			return
		}
	}
	if len(addedTask) > 0 {
		common.SuccessResp(c, fmt.Sprintf("Added %d tasks", len(addedTask)))
	} else {
		common.SuccessResp(c)
	}
}

type RenameReq struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

func FsRename(c *gin.Context) {
	var req RenameReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	if !user.CanRename() {
		common.ErrorResp(c, errs.PermissionDenied, 403)
		return
	}
	reqPath, err := user.JoinPath(req.Path)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	if err := fs.Rename(c, reqPath, req.Name); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c)
}

type RegexRenameReq struct {
	SrcDir       string `json:"src_dir"`
	SrcNameRegex string `json:"src_name_regex"`
	NewNameRegex string `json:"new_name_regex"`
}

func FsRegexRename(c *gin.Context) {
	var req RegexRenameReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	if !user.CanRename() {
		common.ErrorResp(c, errs.PermissionDenied, 403)
		return
	}

	reqPath, err := user.JoinPath(req.SrcDir)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}

	meta, err := op.GetNearestMeta(reqPath)
	if err != nil {
		if !errors.Is(errors.Cause(err), errs.MetaNotFound) {
			common.ErrorResp(c, err, 500, true)
			return
		}
	}
	c.Set("meta", meta)

	srcRegexp, err := regexp.Compile(req.SrcNameRegex)
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}

	files, err := fs.List(c, reqPath, false)
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}

	for _, file := range files {

		if srcRegexp.MatchString(file.GetName()) {
			filePath := fmt.Sprintf("%s/%s", reqPath, file.GetName())
			newFileName := srcRegexp.ReplaceAllString(file.GetName(), req.NewNameRegex)
			if err := fs.Rename(c, filePath, newFileName); err != nil {
				common.ErrorResp(c, err, 500)
				return
			}
		}

	}

	common.SuccessResp(c)
}

type RemoveReq struct {
	Dir   string   `json:"dir"`
	Names []string `json:"names"`
}

func FsRemove(c *gin.Context) {
	var req RemoveReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if len(req.Names) == 0 {
		common.ErrorStrResp(c, "Empty file names", 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	if !user.CanRemove() {
		common.ErrorResp(c, errs.PermissionDenied, 403)
		return
	}
	reqDir, err := user.JoinPath(req.Dir)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	for _, name := range req.Names {
		err := fs.Remove(c, stdpath.Join(reqDir, name))
		if err != nil {
			common.ErrorResp(c, err, 500)
			return
		}
	}
	//fs.ClearCache(req.Dir)
	common.SuccessResp(c)
}

// Link return real link, just for proxy program, it may contain cookie, so just allowed for admin
func Link(c *gin.Context) {
	var req MkdirOrLinkReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	//user := c.MustGet("user").(*model.User)
	//rawPath := stdpath.Join(user.BasePath, req.Path)
	// why need not join base_path? because it's always the full path
	rawPath := req.Path
	storage, err := fs.GetStorage(rawPath)
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	if storage.Config().OnlyLocal {
		common.SuccessResp(c, model.Link{
			URL: fmt.Sprintf("%s/p%s?d&sign=%s",
				common.GetApiUrl(c.Request),
				utils.EncodePath(rawPath, true),
				sign.Sign(rawPath)),
		})
		return
	}
	link, _, err := fs.Link(c, rawPath, model.LinkArgs{IP: c.ClientIP()})
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	if link.Data != nil {
		defer func(Data io.ReadCloser) {
			err := Data.Close()
			if err != nil {
				log.Errorf("close link data error: %v", err)
			}
		}(link.Data)
	}
	common.SuccessResp(c, link)
	return
}
