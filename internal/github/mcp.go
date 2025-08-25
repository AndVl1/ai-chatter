package github

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GitHubMCPClient клиент для работы с GitHub MCP сервером
type GitHubMCPClient struct {
	client  *mcp.Client
	session *mcp.ClientSession
}

// NewGitHubMCPClient создает новый GitHub MCP клиент
func NewGitHubMCPClient() *GitHubMCPClient {
	return &GitHubMCPClient{}
}

// Connect подключается к GitHub MCP серверу через stdio
func (g *GitHubMCPClient) Connect(ctx context.Context, githubToken string) error {
	log.Printf("🔗 Connecting to GitHub MCP server via stdio")

	// Создаем MCP клиент
	g.client = mcp.NewClient(&mcp.Implementation{
		Name:    "ai-chatter-bot-github",
		Version: "1.0.0",
	}, nil)

	// Запускаем GitHub MCP сервер как подпроцесс
	serverPath := "./bin/github-mcp-server"
	if customPath := os.Getenv("GITHUB_MCP_SERVER_PATH"); customPath != "" {
		serverPath = customPath
	}

	log.Printf("🔍 GitHub MCP: Trying to start server at path: %s", serverPath)

	// Логируем текущий рабочий каталог
	if pwd, err := os.Getwd(); err == nil {
		log.Printf("🔍 GitHub MCP: Current working directory: %s", pwd)
	}

	// Проверяем существование файла
	if _, err := os.Stat(serverPath); os.IsNotExist(err) {
		log.Printf("❌ GitHub MCP: Server binary not found at %s", serverPath)
		// Попробуем найти файл в альтернативных местах
		if _, err := os.Stat("./github-mcp-server"); err == nil {
			log.Printf("💡 GitHub MCP: Found server at ./github-mcp-server")
		} else if _, err := os.Stat("/app/github-mcp-server"); err == nil {
			log.Printf("💡 GitHub MCP: Found server at /app/github-mcp-server")
		}
		return fmt.Errorf("github MCP server binary not found at %s", serverPath)
	}

	// Проверяем права на выполнение
	if info, err := os.Stat(serverPath); err == nil {
		log.Printf("🔍 GitHub MCP: Server file exists, mode: %v, size: %d bytes", info.Mode(), info.Size())
	}

	cmd := exec.CommandContext(ctx, serverPath)
	// Передаем GitHub токен
	cmd.Env = append(os.Environ(), fmt.Sprintf("GITHUB_TOKEN=%s", githubToken))

	// Логируем передачу токена
	if githubToken != "" {
		log.Printf("🔑 GitHub MCP: Passing token to subprocess (length: %d)", len(githubToken))
	} else {
		log.Printf("⚠️ GitHub MCP: No token to pass to subprocess")
	}

	transport := mcp.NewCommandTransport(cmd)

	session, err := g.client.Connect(ctx, transport)
	if err != nil {
		return fmt.Errorf("failed to connect to GitHub MCP server: %w", err)
	}

	g.session = session
	log.Printf("✅ Connected to GitHub MCP server")
	return nil
}

// Close закрывает соединение с GitHub MCP сервером
func (g *GitHubMCPClient) Close() error {
	if g.session != nil {
		return g.session.Close()
	}
	return nil
}

// GetReleases получает список релизов репозитория через MCP
func (g *GitHubMCPClient) GetReleases(ctx context.Context, owner, repo string, maxReleases int, includeDrafts, preReleaseOnly bool) GitHubMCPResult {
	if g.session == nil {
		return GitHubMCPResult{Success: false, Message: "GitHub MCP session not connected"}
	}

	log.Printf("📦 Getting GitHub releases via MCP: %s/%s, max=%d, drafts=%v, prerelease=%v", owner, repo, maxReleases, includeDrafts, preReleaseOnly)

	// Вызываем инструмент get_github_releases
	result, err := g.session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_github_releases",
		Arguments: map[string]any{
			"owner":           owner,
			"repo":            repo,
			"max_releases":    maxReleases,
			"include_drafts":  includeDrafts,
			"prerelease_only": preReleaseOnly,
		},
	})

	if err != nil {
		log.Printf("❌ GitHub MCP releases error: %v", err)
		return GitHubMCPResult{Success: false, Message: fmt.Sprintf("GitHub MCP releases error: %v", err)}
	}

	if result.IsError {
		return GitHubMCPResult{Success: false, Message: "GitHub releases tool returned error"}
	}

	// Извлекаем текст из результата
	var responseText string
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			responseText += textContent.Text
		}
	}

	// Извлекаем метаданные с релизами
	var releases []GitHubRelease
	var totalFound int

	if result.Meta != nil {
		// Извлекаем total_found
		if count, ok := result.Meta["total_found"].(float64); ok {
			totalFound = int(count)
		}

		// Извлекаем релизы
		if releasesData, ok := result.Meta["releases"].([]any); ok {
			for _, item := range releasesData {
				if releaseData, ok := item.(map[string]any); ok {
					release := parseGitHubRelease(releaseData)
					releases = append(releases, release)
				}
			}
		}
	}

	return GitHubMCPResult{
		Success:    true,
		Message:    responseText,
		Releases:   releases,
		TotalFound: totalFound,
	}
}

// DownloadAsset скачивает ассет релиза через MCP
func (g *GitHubMCPClient) DownloadAsset(ctx context.Context, owner, repo string, releaseID int64, assetName, targetPath string) GitHubDownloadResult {
	if g.session == nil {
		return GitHubDownloadResult{Success: false, Message: "GitHub MCP session not connected"}
	}

	log.Printf("⬇️ Downloading GitHub asset via MCP: %s/%s, release=%d, asset=%s", owner, repo, releaseID, assetName)

	// Вызываем инструмент download_github_asset
	result, err := g.session.CallTool(ctx, &mcp.CallToolParams{
		Name: "download_github_asset",
		Arguments: map[string]any{
			"owner":       owner,
			"repo":        repo,
			"release_id":  releaseID,
			"asset_name":  assetName,
			"target_path": targetPath,
		},
	})

	if err != nil {
		log.Printf("❌ GitHub MCP download error: %v", err)
		return GitHubDownloadResult{Success: false, Message: fmt.Sprintf("GitHub MCP download error: %v", err)}
	}

	if result.IsError {
		return GitHubDownloadResult{Success: false, Message: "GitHub download tool returned error"}
	}

	// Извлекаем текст из результата
	var responseText string
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			responseText += textContent.Text
		}
	}

	// Извлекаем метаданные
	downloadResult := GitHubDownloadResult{
		Success: true,
		Message: responseText,
	}

	if result.Meta != nil {
		if assetName, ok := result.Meta["asset_name"].(string); ok {
			downloadResult.AssetName = assetName
		}
		if assetSize, ok := result.Meta["asset_size"].(float64); ok {
			downloadResult.AssetSize = int64(assetSize)
		}
		if targetPath, ok := result.Meta["target_path"].(string); ok {
			downloadResult.TargetPath = targetPath
		}
		if contentType, ok := result.Meta["content_type"].(string); ok {
			downloadResult.ContentType = contentType
		}
		if base64Content, ok := result.Meta["base64_content"].(string); ok {
			downloadResult.Base64Content = base64Content
		}
		if releaseData, ok := result.Meta["release"].(map[string]any); ok {
			downloadResult.Release = parseGitHubRelease(releaseData)
		}
	}

	return downloadResult
}

// GetLatestPreRelease получает последний pre-release
func (g *GitHubMCPClient) GetLatestPreRelease(ctx context.Context, owner, repo string) (*GitHubRelease, error) {
	result := g.GetReleases(ctx, owner, repo, 10, false, true)
	if !result.Success {
		return nil, fmt.Errorf("failed to get pre-releases: %s", result.Message)
	}

	if len(result.Releases) == 0 {
		return nil, fmt.Errorf("no pre-releases found")
	}

	// Возвращаем первый (самый свежий) pre-release
	return &result.Releases[0], nil
}

// FindAABAsset ищет AAB файл среди ассетов релиза
func (g *GitHubMCPClient) FindAABAsset(release GitHubRelease) *GitHubReleaseAsset {
	for _, asset := range release.Assets {
		if isAABFile(asset.Name) {
			return &asset
		}
	}
	return nil
}

// FindAPKAsset ищет APK файл среди ассетов релиза
func (g *GitHubMCPClient) FindAPKAsset(release GitHubRelease) *GitHubReleaseAsset {
	for _, asset := range release.Assets {
		if isAPKFile(asset.Name) {
			return &asset
		}
	}
	return nil
}

// FindAndroidAsset ищет сначала AAB, потом APK файл среди ассетов релиза
func (g *GitHubMCPClient) FindAndroidAsset(release GitHubRelease) *GitHubReleaseAsset {
	// Сначала ищем AAB (предпочтительно)
	if aabAsset := g.FindAABAsset(release); aabAsset != nil {
		return aabAsset
	}

	// Fallback на APK
	if apkAsset := g.FindAPKAsset(release); apkAsset != nil {
		return apkAsset
	}

	return nil
}

// isAABFile проверяет, является ли файл AAB
func isAABFile(filename string) bool {
	return len(filename) > 4 && filename[len(filename)-4:] == ".aab"
}

// isAPKFile проверяет, является ли файл APK
func isAPKFile(filename string) bool {
	return len(filename) > 4 && filename[len(filename)-4:] == ".apk"
}

// GetAssetType возвращает тип файла (AAB или APK)
func GetAssetType(filename string) string {
	if isAABFile(filename) {
		return "AAB"
	}
	if isAPKFile(filename) {
		return "APK"
	}
	return "Unknown"
}

// parseGitHubRelease парсит данные релиза из map
func parseGitHubRelease(data map[string]any) GitHubRelease {
	release := GitHubRelease{}

	if id, ok := data["id"].(float64); ok {
		release.ID = int64(id)
	}
	if tagName, ok := data["tag_name"].(string); ok {
		release.TagName = tagName
	}
	if name, ok := data["name"].(string); ok {
		release.Name = name
	}
	if body, ok := data["body"].(string); ok {
		release.Body = body
	}
	if isDraft, ok := data["draft"].(bool); ok {
		release.IsDraft = isDraft
	}
	if isPrerelease, ok := data["prerelease"].(bool); ok {
		release.IsPrerelease = isPrerelease
	}
	if htmlURL, ok := data["html_url"].(string); ok {
		release.HTMLURL = htmlURL
	}

	// Парсим даты
	if createdAt, ok := data["created_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			release.CreatedAt = t
		}
	}
	if publishedAt, ok := data["published_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, publishedAt); err == nil {
			release.PublishedAt = t
		}
	}

	// Парсим автора
	if authorData, ok := data["author"].(map[string]any); ok {
		if id, ok := authorData["id"].(float64); ok {
			release.Author.ID = int64(id)
		}
		if login, ok := authorData["login"].(string); ok {
			release.Author.Login = login
		}
		if avatarURL, ok := authorData["avatar_url"].(string); ok {
			release.Author.AvatarURL = avatarURL
		}
		if htmlURL, ok := authorData["html_url"].(string); ok {
			release.Author.HTMLURL = htmlURL
		}
	}

	// Парсим ассеты
	if assetsData, ok := data["assets"].([]any); ok {
		for _, assetData := range assetsData {
			if assetMap, ok := assetData.(map[string]any); ok {
				asset := GitHubReleaseAsset{}

				if id, ok := assetMap["id"].(float64); ok {
					asset.ID = int64(id)
				}
				if name, ok := assetMap["name"].(string); ok {
					asset.Name = name
				}
				if label, ok := assetMap["label"].(string); ok {
					asset.Label = label
				}
				if size, ok := assetMap["size"].(float64); ok {
					asset.Size = int64(size)
				}
				if downloadURL, ok := assetMap["browser_download_url"].(string); ok {
					asset.DownloadURL = downloadURL
				}
				if contentType, ok := assetMap["content_type"].(string); ok {
					asset.ContentType = contentType
				}
				if state, ok := assetMap["state"].(string); ok {
					asset.State = state
				}

				// Парсим даты ассета
				if createdAt, ok := assetMap["created_at"].(string); ok {
					if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
						asset.CreatedAt = t
					}
				}
				if updatedAt, ok := assetMap["updated_at"].(string); ok {
					if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
						asset.UpdatedAt = t
					}
				}

				release.Assets = append(release.Assets, asset)
			}
		}
	}

	return release
}

// Структуры данных

// GitHubMCPResult результат GitHub MCP операции
type GitHubMCPResult struct {
	Success    bool            `json:"success"`
	Message    string          `json:"message"`
	Releases   []GitHubRelease `json:"releases"`
	TotalFound int             `json:"total_found"`
}

// GitHubDownloadResult результат скачивания ассета
type GitHubDownloadResult struct {
	Success       bool          `json:"success"`
	Message       string        `json:"message"`
	AssetName     string        `json:"asset_name"`
	AssetSize     int64         `json:"asset_size"`
	TargetPath    string        `json:"target_path"`
	ContentType   string        `json:"content_type"`
	Base64Content string        `json:"base64_content"`
	Release       GitHubRelease `json:"release"`
}

// GitHubRelease информация о релизе GitHub
type GitHubRelease struct {
	ID           int64                `json:"id"`
	TagName      string               `json:"tag_name"`
	Name         string               `json:"name"`
	Body         string               `json:"body"`
	IsDraft      bool                 `json:"draft"`
	IsPrerelease bool                 `json:"prerelease"`
	CreatedAt    time.Time            `json:"created_at"`
	PublishedAt  time.Time            `json:"published_at"`
	Assets       []GitHubReleaseAsset `json:"assets"`
	Author       GitHubUser           `json:"author"`
	HTMLURL      string               `json:"html_url"`
}

// GitHubReleaseAsset информация об ассете релиза
type GitHubReleaseAsset struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Label       string    `json:"label"`
	Size        int64     `json:"size"`
	DownloadURL string    `json:"browser_download_url"`
	ContentType string    `json:"content_type"`
	State       string    `json:"state"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// GitHubUser информация о пользователе GitHub
type GitHubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
	HTMLURL   string `json:"html_url"`
}

// DecodeBase64Content декодирует base64 содержимое в байты
func (r *GitHubDownloadResult) DecodeBase64Content() ([]byte, error) {
	if r.Base64Content == "" {
		return nil, fmt.Errorf("no base64 content available")
	}
	return base64.StdEncoding.DecodeString(r.Base64Content)
}
