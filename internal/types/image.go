package types

import (
	"context"
	"fmt"
	"log"
	"monica-proxy/internal/config"
	"monica-proxy/internal/utils"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

const MaxFileSize = 10 * 1024 * 1024 // 10MB

// imageCache 用于缓存已上传的图片信息
var imageCache = make(map[string]*FileInfo)

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
	_, err = utils.RestyDefaultClient.R().
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
	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file failed: %v", err)
	}

	_, err = utils.RestyDefaultClient.R().
		SetHeader("Content-Type", "image/png").
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
	_, err = utils.RestyDefaultClient.R().
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

// UploadBase64Image 上传base64编码的图片到Monica
func UploadBase64Image(ctx context.Context, base64Data string) (*FileInfo, error) {
	// 1. 检查缓存
	if fileInfo, exists := imageCache[base64Data]; exists {
		return fileInfo, nil
	}

	// 2. 解析base64数据
	// 移除 "data:image/png;base64," 这样的前缀
	parts := strings.Split(base64Data, ",")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid base64 image format")
	}

	// 获取图片类型
	mimeType := strings.TrimSuffix(strings.TrimPrefix(parts[0], "data:"), ";base64")
	if !strings.HasPrefix(mimeType, "image/") {
		return nil, fmt.Errorf("invalid image mime type: %s", mimeType)
	}

	// 解码base64数据
	imageData, err := utils.Base64Decode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode base64 failed: %v", err)
	}

	// 3. 验证图片格式和大小
	fileInfo, err := validateImageBytes(imageData, mimeType)
	if err != nil {
		return nil, fmt.Errorf("validate image failed: %v", err)
	}
	log.Printf("file info: %+v", fileInfo)

	// 4. 获取预签名URL
	preSignReq := &PreSignRequest{
		FilenameList: []string{fileInfo.FileName},
		Module:       ImageModule,
		Location:     ImageLocation,
		ObjID:        uuid.New().String(),
	}

	var preSignResp PreSignResponse
	_, err = utils.RestyDefaultClient.R().
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
	log.Printf("preSign info: %+v", preSignResp)

	// 5. 上传图片数据
	_, err = utils.RestyDefaultClient.R().
		SetContext(ctx).
		SetHeader("Content-Type", fileInfo.FileType).
		SetBody(imageData).
		Put(preSignResp.Data.PreSignURLList[0])

	if err != nil {
		return nil, fmt.Errorf("upload file failed: %v", err)
	}

	// 6. 创建文件对象
	fileInfo.ObjectURL = preSignResp.Data.ObjectURLList[0]
	uploadReq := &FileUploadRequest{
		Data: []FileInfo{*fileInfo},
	}

	var uploadResp FileUploadResponse
	_, err = utils.RestyDefaultClient.R().
		SetContext(ctx).
		SetHeader("cookie", config.MonicaConfig.MonicaCookie).
		SetBody(uploadReq).
		SetResult(&uploadResp).
		Post(FileUploadURL)

	if err != nil {
		return nil, fmt.Errorf("create file object failed: %v", err)
	}
	log.Printf("uploadResp: %+v", uploadResp)
	if len(uploadResp.Data.Items) > 0 {
		fileInfo.FileName = uploadResp.Data.Items[0].FileName
		fileInfo.FileType = uploadResp.Data.Items[0].FileType
		fileInfo.FileSize = uploadResp.Data.Items[0].FileSize
		fileInfo.FileUID = uploadResp.Data.Items[0].FileUID
		fileInfo.FileExt = uploadResp.Data.Items[0].FileType
		fileInfo.FileTokens = uploadResp.Data.Items[0].FileTokens
		fileInfo.FileChunks = uploadResp.Data.Items[0].FileChunks
	}

	fileInfo.UseFullText = true
	fileInfo.FileURL = preSignResp.Data.CDNURLList[0]

	// 7. 获取文件llm读取结果知道有返回
	var batchResp FileBatchGetResponse
	reqMap := make(map[string][]string)
	reqMap["file_uids"] = []string{fileInfo.FileUID}
	var retryCount = 0
	for {
		if retryCount > 2 {
			return nil, fmt.Errorf("retry limit exceeded")
		}
		_, err = utils.RestyDefaultClient.R().
			SetContext(ctx).
			SetHeader("cookie", config.MonicaConfig.MonicaCookie).
			SetBody(reqMap).
			SetResult(&batchResp).
			Post(FileGetURL)
		if err != nil {
			return nil, fmt.Errorf("batch get file failed: %v", err)
		}
		if len(batchResp.Data.Items) > 0 && batchResp.Data.Items[0].FileChunks > 0 {
			break
		} else {
			time.Sleep(1 * time.Second)
			retryCount++
		}
	}
	fileInfo.FileChunks = batchResp.Data.Items[0].FileChunks
	fileInfo.FileTokens = batchResp.Data.Items[0].FileTokens
	fileInfo.URL = ""
	fileInfo.ObjectURL = ""

	// 8. 保存到缓存
	imageCache[base64Data] = fileInfo

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

// validateImageBytes 验证图片字节数据的格式和大小
func validateImageBytes(imageData []byte, mimeType string) (*FileInfo, error) {
	if len(imageData) > MaxFileSize {
		return nil, fmt.Errorf("file size exceeds limit: %d > %d", len(imageData), MaxFileSize)
	}

	contentType := http.DetectContentType(imageData)
	if !SupportedImageTypes[contentType] {
		return nil, fmt.Errorf("unsupported image type: %s", contentType)
	}

	// 根据MIME类型生成文件扩展名
	ext := ".png"
	switch mimeType {
	case "image/jpeg":
		ext = ".jpg"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	}

	fileName := fmt.Sprintf("%s%s", uuid.New().String(), ext)

	return &FileInfo{
		FileName: fileName,
		FileSize: int64(len(imageData)),
		FileType: contentType,
	}, nil
}