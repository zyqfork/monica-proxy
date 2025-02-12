package types

import (
	"context"
	"fmt"
	"log"
	"strings"

	lop "github.com/samber/lo/parallel"

	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
)

//const (
//	MonicaModelGPT4o        = "gpt-4o"
//	MonicaModelGPT4oMini    = "gpt-4o-mini"
//	MonicaModelClaudeSonnet = "claude-3"
//	MonicaModelClaudeHaiku  = "claude-3.5-haiku"
//	MonicaModelGemini2      = "gemini_2_0"
//	MonicaModelO1Preview    = "openai_o_1"
//	MonicaModelO1Mini       = "openai-o-1-mini"
//)

const (
	BotChatURL    = "https://api.monica.im/api/custom_bot/chat"
	PreSignURL    = "https://api.monica.im/api/file_object/pre_sign_list_by_module"
	FileUploadURL = "https://api.monica.im/api/files/batch_create_llm_file"
	FileGetURL    = "https://api.monica.im/api/files/batch_get_file"
)

// 图片相关常量
const (
	MaxImageSize  = 10 * 1024 * 1024 // 10MB
	ImageModule   = "chat_bot"
	ImageLocation = "files"
)

// 支持的图片格式
var SupportedImageTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

type ChatGPTRequest struct {
	Model    string        `json:"model"`    // gpt-3.5-turbo, gpt-4, ...
	Messages []ChatMessage `json:"messages"` // 对话数组
	Stream   bool          `json:"stream"`   // 是否流式返回
}

type ChatMessage struct {
	Role    string      `json:"role"`    // "system", "user", "assistant"
	Content interface{} `json:"content"` // 可以是字符串或MessageContent数组
}

// MessageContent 消息内容
type MessageContent struct {
	Type     string `json:"type"`                // "text" 或 "image_url"
	Text     string `json:"text,omitempty"`      // 文本内容
	ImageURL string `json:"image_url,omitempty"` // 图片URL
}

// MonicaRequest 为 Monica 自定义 AI 的请求格式
// 注意：以下字段仅示例。真正要与 Monica 对接时，请根据其 API 要求调整字段。
type MonicaRequest struct {
	TaskUID  string    `json:"task_uid"`
	BotUID   string    `json:"bot_uid"`
	Data     DataField `json:"data"`
	Language string    `json:"language"`
	TaskType string    `json:"task_type"`
	ToolData ToolData  `json:"tool_data"`
}

// DataField 在 Monica 的 body 中
type DataField struct {
	ConversationID  string `json:"conversation_id"`
	PreParentItemID string `json:"pre_parent_item_id"`
	Items           []Item `json:"items"`
	TriggerBy       string `json:"trigger_by"`
	UseModel        string `json:"use_model,omitempty"`
	IsIncognito     bool   `json:"is_incognito"`
	UseNewMemory    bool   `json:"use_new_memory"`
}

type Item struct {
	ConversationID string      `json:"conversation_id"`
	ParentItemID   string      `json:"parent_item_id,omitempty"`
	ItemID         string      `json:"item_id"`
	ItemType       string      `json:"item_type"`
	Data           ItemContent `json:"data"`
}

type ItemContent struct {
	Type                   string     `json:"type"`
	Content                string     `json:"content"`
	MaxToken               int        `json:"max_token,omitempty"`
	IsIncognito            bool       `json:"is_incognito,omitempty"` // 是否无痕模式
	FromTaskType           string     `json:"from_task_type,omitempty"`
	ManualWebSearchEnabled bool       `json:"manual_web_search_enabled,omitempty"` // 网页搜索
	UseModel               string     `json:"use_model,omitempty"`
	FileInfos              []FileInfo `json:"file_infos,omitempty"`
}

// ToolData 这里演示放空
type ToolData struct {
	SysSkillList []string `json:"sys_skill_list"`
}

// PreSignRequest 预签名请求
type PreSignRequest struct {
	FilenameList []string `json:"filename_list"`
	Module       string   `json:"module"`
	Location     string   `json:"location"`
	ObjID        string   `json:"obj_id"`
}

// PreSignResponse 预签名响应
type PreSignResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		PreSignURLList []string `json:"pre_sign_url_list"`
		ObjectURLList  []string `json:"object_url_list"`
		CDNURLList     []string `json:"cdn_url_list"`
	} `json:"data"`
}

// FileInfo 文件信息
type FileInfo struct {
	URL        string `json:"url,omitempty"`
	FileURL    string `json:"file_url"`
	FileUID    string `json:"file_uid"`
	Parse      bool   `json:"parse"`
	FileName   string `json:"file_name"`
	FileSize   int64  `json:"file_size"`
	FileType   string `json:"file_type"`
	FileExt    string `json:"file_ext"`
	FileTokens int64  `json:"file_tokens"`
	FileChunks int64  `json:"file_chunks"`
	ObjectURL  string `json:"object_url,omitempty"`
	//Embedding    bool                   `json:"embedding"`
	FileMetaInfo map[string]interface{} `json:"file_meta_info,omitempty"`
	UseFullText  bool                   `json:"use_full_text"`
}

// FileUploadRequest 文件上传请求
type FileUploadRequest struct {
	Data []FileInfo `json:"data"`
}

// FileUploadResponse 文件上传响应
type FileUploadResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Items []struct {
			FileName   string `json:"file_name"`
			FileType   string `json:"file_type"`
			FileSize   int64  `json:"file_size"`
			FileUID    string `json:"file_uid"`
			FileTokens int64  `json:"file_tokens"`
			FileChunks int64  `json:"file_chunks"`
			// 其他字段暂时不需要
		} `json:"items"`
	} `json:"data"`
}

// FileBatchGetResponse 获取文件llm处理是否完成
type FileBatchGetResponse struct {
	Data struct {
		Items []struct {
			FileName     string `json:"file_name"`
			FileType     string `json:"file_type"`
			FileSize     int    `json:"file_size"`
			ObjectUrl    string `json:"object_url"`
			Url          string `json:"url"`
			FileMetaInfo struct {
			} `json:"file_meta_info"`
			DriveFileUid  string `json:"drive_file_uid"`
			FileUid       string `json:"file_uid"`
			IndexState    int    `json:"index_state"`
			IndexDesc     string `json:"index_desc"`
			ErrorMessage  string `json:"error_message"`
			FileTokens    int64  `json:"file_tokens"`
			FileChunks    int64  `json:"file_chunks"`
			IndexProgress int    `json:"index_progress"`
		} `json:"items"`
	} `json:"data"`
}

// OpenAIModel represents a model in the OpenAI API format
type OpenAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

// OpenAIModelList represents the response format for the /v1/models endpoint
type OpenAIModelList struct {
	Object string        `json:"object"`
	Data   []OpenAIModel `json:"data"`
}

// GetSupportedModels returns all supported models in OpenAI format
func GetSupportedModels() OpenAIModelList {
	models := []OpenAIModel{
		{ID: "gpt-4o-mini", Object: "model", OwnedBy: "monica"},
		{ID: "gpt-4o", Object: "model", OwnedBy: "monica"},
		{ID: "claude-3-5-sonnet", Object: "model", OwnedBy: "monica"},
		{ID: "claude-3-5-haiku", Object: "model", OwnedBy: "monica"},
		{ID: "gemini-2.0-pro", Object: "model", OwnedBy: "monica"},
		{ID: "gemini-2.0-flash", Object: "model", OwnedBy: "monica"},
		{ID: "gemini-1.5-pro", Object: "model", OwnedBy: "monica"},
		{ID: "o3-mini", Object: "model", OwnedBy: "monica"},
		{ID: "o1-preview", Object: "model", OwnedBy: "monica"},
		{ID: "deepseek-reasoner", Object: "model", OwnedBy: "monica"},
		{ID: "deepseek-chat", Object: "model", OwnedBy: "monica"},
	}

	return OpenAIModelList{
		Object: "list",
		Data:   models,
	}
}

// ChatGPTToMonica 将 ChatGPTRequest 转换为 MonicaRequest
func ChatGPTToMonica(chatReq openai.ChatCompletionRequest) (*MonicaRequest, error) {
	if len(chatReq.Messages) == 0 {
		return nil, fmt.Errorf("empty messages")
	}

	// 生成会话ID
	conversationID := fmt.Sprintf("conv:%s", uuid.New().String())

	// 转换消息

	// 设置默认欢迎消息头，不加上就有几率去掉问题最后的十几个token，不清楚是不是bug
	defaultItem := Item{
		ItemID:         fmt.Sprintf("msg:%s", uuid.New().String()),
		ConversationID: conversationID,
		ItemType:       "reply",
		Data:           ItemContent{Type: "text", Content: "__RENDER_BOT_WELCOME_MSG__"},
	}
	var items = make([]Item, 1, len(chatReq.Messages))
	items[0] = defaultItem
	preItemID := defaultItem.ItemID

	for _, msg := range chatReq.Messages {
		if msg.Role == "system" {
			// monica不支持设置prompt，所以直接跳过
			continue
		}
		var msgContext string
		var imgUrl []*openai.ChatMessageImageURL
		if len(msg.MultiContent) > 0 { // 说明应该是多内容，可能是图片内容
			for _, content := range msg.MultiContent {
				switch content.Type {
				case "text":
					msgContext = content.Text
				case "image_url":
					imgUrl = append(imgUrl, content.ImageURL)
				}
			}
		}
		itemID := fmt.Sprintf("msg:%s", uuid.New().String())
		itemType := "question"
		if msg.Role == "assistant" {
			itemType = "reply"
		}

		var content ItemContent
		if len(imgUrl) > 0 {
			ctx := context.Background()
			fileIfoList := lop.Map(imgUrl, func(item *openai.ChatMessageImageURL, _ int) FileInfo {
				f, err := UploadBase64Image(ctx, item.URL)
				if err != nil {
					log.Println(err)
					return FileInfo{}
				}
				return *f
			})

			content = ItemContent{
				Type:        "file_with_text",
				Content:     msgContext,
				FileInfos:   fileIfoList,
				IsIncognito: true,
			}
		} else {
			content = ItemContent{
				Type:        "text",
				Content:     msg.Content,
				IsIncognito: true,
			}
		}

		item := Item{
			ConversationID: conversationID,
			ItemID:         itemID,
			ParentItemID:   preItemID,
			ItemType:       itemType,
			Data:           content,
		}
		items = append(items, item)
		preItemID = itemID
	}

	// 构建请求
	mReq := &MonicaRequest{
		TaskUID: fmt.Sprintf("task:%s", uuid.New().String()),
		BotUID:  modelToBot(chatReq.Model),
		Data: DataField{
			ConversationID:  conversationID,
			Items:           items,
			PreParentItemID: preItemID,
			TriggerBy:       "auto",
			IsIncognito:     true,
			UseModel:        "", //TODO 好像写啥都没影响
			UseNewMemory:    false,
		},
		Language: "auto",
		TaskType: "chat",
	}

	//indent, err := json.MarshalIndent(mReq, "", "  ")
	//if err != nil {
	//	return nil, err
	//} else {
	//	log.Printf("send: \n%s\n", indent)
	//}

	return mReq, nil
}

func modelToBot(model string) string {
	switch {
	case strings.HasPrefix(model, "gpt-4o-mini"):
		return "gpt_4_o_mini_chat"
	case strings.HasPrefix(model, "gpt-4o"):
		return "gpt_4_o_chat"
	case strings.HasPrefix(model, "claude-3-5-sonnet"):
		return "claude_3.5_sonnet"
	case strings.Contains(model, "haiku"):
		return "claude_3.5_haiku"
	case strings.HasPrefix(model, "gemini-2"):
		return "gemini_2_0"
	case strings.HasPrefix(model, "gemini-1"):
		return "gemini_1_5"
	case strings.HasPrefix(model, "o1-mini"):
		return "openai_o_1_mini"
	case strings.HasPrefix(model, "o1-preview"):
		return "openai_o_1"
	case model == "deepseek-reasoner":
		return "deepseek_reasoner"
	case model == "deepseek-chat":
		return "deepseek_chat"
	default:
		return "claude_3.5_sonnet"
	}
}
