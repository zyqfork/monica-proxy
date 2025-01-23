package types

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"io/ioutil"
	"monica-proxy/internal/config"
	"monica-proxy/internal/utils"
	"net/http"
	"os"
	"strings"
)

// UploadImage 上传图片到Monica
func UploadImage(ctx context.Context, filePath string) (*FileInfo, error) {
	// 1. 验证文件格式和大小
	fileInfo, err := validateImage(filePath)
	if err != nil {
		return nil, fmt.Errorf("validate image failed: %v", err)
	}

	// 2. 获取预签名URL
	preSignReq := &PreSignRequest{
		FilenameList: []string{fileInfo.FileName},
		Module:       ImageModule,
		Location:     ImageLocation,
		ObjID:        uuid.New().String(),
	}

	var preSignResp PreSignResponse
	_, err = utils.RestyClient.R().
		SetContext(ctx).
		SetHeader("cookie", config.MonicaConfig.MonicaCookie).
		SetBody(preSignReq).
		SetResult(&preSignResp).
		Post(PreSignURL)

	if err != nil {
		return nil, fmt.Errorf("get pre-sign url failed: %v", err)
	}

	if len(preSignResp.Data.PreSignURLList) == 0 || len(preSignResp.Data.ObjectURLList) == 0 {
		return nil, fmt.Errorf("no pre-sign url or object url returned")
	}

	// 3. 上传文件到预签名URL
	fileBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file failed: %v", err)
	}

	_, err = utils.RestyClient.R().
		SetContext(ctx).
		SetBody(fileBytes).
		Put(preSignResp.Data.PreSignURLList[0])

	if err != nil {
		return nil, fmt.Errorf("upload file failed: %v", err)
	}

	// 4. 创建文件对象
	fileInfo.ObjectURL = preSignResp.Data.ObjectURLList[0]
	uploadReq := &FileUploadRequest{
		Data: []FileInfo{*fileInfo},
	}

	var uploadResp FileUploadResponse
	_, err = utils.RestyClient.R().
		SetContext(ctx).
		SetHeader("cookie", config.MonicaConfig.MonicaCookie).
		SetBody(uploadReq).
		SetResult(&uploadResp).
		Post(FileUploadURL)

	if err != nil {
		return nil, fmt.Errorf("create file object failed: %v", err)
	}

	return fileInfo, nil
}

// validateImage 验证图片格式和大小
func validateImage(filePath string) (*FileInfo, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file failed: %v", err)
	}
	defer file.Close()

	// 读取文件头以判断类型
	buffer := make([]byte, 512)
	_, err = file.Read(buffer)
	if err != nil {
		return nil, fmt.Errorf("read file header failed: %v", err)
	}

	contentType := http.DetectContentType(buffer)
	if !SupportedImageTypes[contentType] {
		return nil, fmt.Errorf("unsupported image type: %s", contentType)
	}

	// 获取文件信息
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("get file info failed: %v", err)
	}

	if fileInfo.Size() > MaxImageSize {
		return nil, fmt.Errorf("file size exceeds limit: %d > %d", fileInfo.Size(), MaxImageSize)
	}

	return &FileInfo{
		FileName: fileInfo.Name(),
		FileSize: fileInfo.Size(),
		FileType: strings.TrimPrefix(contentType, "image/"),
		Parse:    true,
	}, nil
}