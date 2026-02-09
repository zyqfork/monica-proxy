package types

import (
	"context"
	"fmt"
	"log"
	"monica-proxy/internal/config"
	"sync"

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
	ConversationID      string `json:"conversation_id"`
	Items               []Item `json:"items"`
	PreGeneratedReplyId string `json:"pre_generated_reply_id"`
	PreParentItemID     string `json:"pre_parent_item_id"`
	Origin              string `json:"origin"`
	OriginPageTitle     string `json:"origin_page_title"`
	TriggerBy           string `json:"trigger_by"`
	UseModel            string `json:"use_model,omitempty"`
	IsIncognito         bool   `json:"is_incognito"`
	UseNewMemory        bool   `json:"use_new_memory"`
	UseMemorySuggestion bool   `json:"use_memory_suggestion"`
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
	ID              string `json:"id"`
	Object          string `json:"object"`
	BotUid          string `json:"-"`
	Origin          string `json:"-"`
	OriginPageTitle string `json:"-"`
	OwnedBy         string `json:"owned_by"`
}

// OpenAIModelList represents the response format for the /v1/models endpoint
type OpenAIModelList struct {
	Object string        `json:"object"`
	Data   []OpenAIModel `json:"data"`
}

var modelMap = map[string]OpenAIModel{
	"gpt-5":                      {Object: "model", OwnedBy: "monica", BotUid: "gpt_5", Origin: "https://monica.im/home/chat/GPT-5/gpt_5", OriginPageTitle: "GPT-5 - Monica 智能体"},
	"gpt-4o":                     {Object: "model", OwnedBy: "monica", BotUid: "gpt_4_o_mini_chat", Origin: "https://monica.im/home/chat/gpt-4o/gpt_4_o_chat", OriginPageTitle: "GPT-4o - Monica 智能体"},
	"gpt-4.1":                    {Object: "model", OwnedBy: "monica", BotUid: "gpt_4_1", Origin: "https://monica.im/home/chat/GPT-4.1/gpt_4_1", OriginPageTitle: "GPT-4.1 - Monica 智能体"},
	"gpt-4.5-preview":            {Object: "model", OwnedBy: "monica", BotUid: "gpt_4_5_chat", Origin: "https://monica.im/home/chat/GPT-4.5/gpt_4_5_chat", OriginPageTitle: "GPT-4.5 - Monica 智能体"},
	"openai-o1":                  {Object: "model", OwnedBy: "monica", BotUid: "openai_o1", Origin: "https://monica.im/home/chat/o1/openai_o1", OriginPageTitle: "o1 - Monica 智能体"},
	"openai-o-3-mini":            {Object: "model", OwnedBy: "monica", BotUid: "openai_o_3_mini", Origin: "https://monica.im/home/chat/o3-mini/openai_o_3_mini", OriginPageTitle: "o3-mini - Monica 智能体"},
	"gpt-4o-mini":                {Object: "model", OwnedBy: "monica", BotUid: "gpt_4_o_mini_chat", Origin: "https://monica.im/home/chat/gpt-4o-mini/gpt_4_o_mini_chat", OriginPageTitle: "GPT-4o mini - Monica 智能体"},
	"grok-3-beta":                {Object: "model", OwnedBy: "monica", BotUid: "grok_3_beta", Origin: "https://monica.im/home/chat/Grok%203/grok_3_beta", OriginPageTitle: "Grok 3 - Monica 智能体"},
	"claude-3.5-haiku":           {Object: "model", OwnedBy: "monica", BotUid: "claude_3.5_haiku", Origin: "https://monica.im/home/chat/Claude%203.5%20Haiku/claude_3.5_haiku", OriginPageTitle: "Claude 3.5 Haiku - Monica 智能体"},
	"claude-3.5-sonnet":          {Object: "model", OwnedBy: "monica", BotUid: "claude_3.5_sonnet", Origin: "https://monica.im/home/chat/Claude%203.5%20Sonnet%20V2/claude_3.5_sonnet", OriginPageTitle: "Claude 3.5 Sonnet V2 - Monica 智能体"},
	"claude-3.7-sonnet":          {Object: "model", OwnedBy: "monica", BotUid: "claude_3_7_sonnet", Origin: "https://monica.im/home/chat/Claude%203.7%20Sonnet/claude_3_7_sonnet", OriginPageTitle: "Claude 3.7 Sonnet - Monica 智能体"},
	"claude-3.7-sonnet-thinking": {Object: "model", OwnedBy: "monica", BotUid: "claude_3_7_sonnet_think", Origin: "https://monica.im/home/chat/Claude%203.7%20Sonnet%20Thinking/claude_3_7_sonnet_think", OriginPageTitle: "Claude 3.7 Sonnet Thinking - Monica 智能体"},
	"claude-4-sonnet":            {Object: "model", OwnedBy: "monica", BotUid: "claude_4_sonnet", Origin: "https://monica.im/home/chat/claude-4-sonnet/claude_4_sonnet", OriginPageTitle: "Claude 4 Sonnet - Monica 智能体"},
	"claude-4-opus":              {Object: "model", OwnedBy: "monica", BotUid: "claude_4_opus", Origin: "https://monica.im/home/chat/Claude%204%20Opus/claude_4_opus", OriginPageTitle: "Claude 4 Opus - Monica 智能体"},
	"claude-sonnet-4-5":          {Object: "model", OwnedBy: "monica", BotUid: "claude_4_5_sonnet", Origin: "https://monica.im/home/chat/Claude%204.5%20Sonnet/claude_4_5_sonnet", OriginPageTitle: "Claude 4.5 Sonnet - Monica 智能体"},
	"deepclaude":                 {Object: "model", OwnedBy: "monica", BotUid: "deepclaude", Origin: "https://monica.im/home/chat/DeepClaude/deepclaude", OriginPageTitle: "DeepClaude - Monica 智能体"},
	"gemini-2.5-pro":             {Object: "model", OwnedBy: "monica", BotUid: "gemini_2_5_pro", Origin: "https://monica.im/home/chat/Gemini%202.5%20Pro/gemini_2_5_pro", OriginPageTitle: "Gemini 2.5 Pro - Monica 智能体"},
	"gemini-2.5-flash":           {Object: "model", OwnedBy: "monica", BotUid: "gemini_2_5_flash", Origin: "https://monica.im/home/chat/Gemini%202.5%20Flash/gemini_2_5_flash", OriginPageTitle: "Gemini 2.5 Flash - Monica 智能体"},
	"deepseek-chat":              {Object: "model", OwnedBy: "monica", BotUid: "deepseek_chat", Origin: "https://monica.im/home/chat/DeepSeek%20V3/deepseek_chat", OriginPageTitle: "DeepSeek V3 - Monica 智能体"},
	"deepseek-reasoner":          {Object: "model", OwnedBy: "monica", BotUid: "deepseek_reasoner", Origin: "https://monica.im/home/chat/DeepSeek%20R1/deepseek_reasoner", OriginPageTitle: "DeepSeek R1 - Monica 智能体"},
	"llama-3.3-70b":              {Object: "model", OwnedBy: "monica", BotUid: "llama_3_3_70b", Origin: "https://monica.im/home/chat/Llama%203.3%2070B/llama_3_3_70b", OriginPageTitle: "Llama 3.3 70B - Monica 智能体"},
	"llama-3.1-405b":             {Object: "model", OwnedBy: "monica", BotUid: "llama_3_1_405b", Origin: "https://monica.im/home/chat/Llama%203.1%20405B/llama_3_1_405b", OriginPageTitle: "Llama 3.1 405B - Monica 智能体"},
}

func IsModelSupported(modelName string) bool {
	_, exists := modelMap[modelName]
	return exists
}

var (
	cachedModels     OpenAIModelList
	cachedModelsOnce sync.Once
)

func GetSupportedModels() OpenAIModelList {
	cachedModelsOnce.Do(func() {
		modelSlice := make([]OpenAIModel, 0, len(modelMap))
		for id, model := range modelMap {
			modelWithID := model
			modelWithID.ID = id
			modelSlice = append(modelSlice, modelWithID)
		}
		cachedModels = OpenAIModelList{
			Object: "list",
			Data:   modelSlice,
		}
	})
	return cachedModels
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
	preReplyID := fmt.Sprintf("msg:%s", uuid.New().String())

	system_prompt := ""
	for _, msg := range chatReq.Messages {
		if msg.Role == "system" {
			//monica不支持系统提示词，拼接到用户提示词前面，实现系统提示词效果
			system_prompt = msg.Content + "\n"
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
		} else {
			//拼接用户提示词到系统提示词前面
			msg.Content = system_prompt + msg.Content
			system_prompt = ""
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
				IsIncognito: config.MonicaConfig.IsIncognito,
			}
		} else {
			content = ItemContent{
				Type:        "text",
				Content:     msg.Content,
				IsIncognito: config.MonicaConfig.IsIncognito,
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
		BotUID:  modelMap[chatReq.Model].BotUid,
		Data: DataField{
			ConversationID:      conversationID,
			Items:               items,
			PreGeneratedReplyId: preReplyID,
			PreParentItemID:     preItemID,
			Origin:              modelMap[chatReq.Model].Origin,
			OriginPageTitle:     modelMap[chatReq.Model].OriginPageTitle,
			TriggerBy:           "auto",
			IsIncognito:         config.MonicaConfig.IsIncognito,
			UseModel:            chatReq.Model,
			UseNewMemory:        false,
			UseMemorySuggestion: false,
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
