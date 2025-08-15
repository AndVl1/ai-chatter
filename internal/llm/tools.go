package llm

// GetNotionTools возвращает список инструментов Notion для LLM
func GetNotionTools() []Tool {
	return []Tool{
		{
			Type: "function",
			Function: Function{
				Name:        "save_dialog_to_notion",
				Description: "Сохраняет текущий диалог в Notion. Используется когда пользователь просит сохранить беседу, запомнить информацию или создать заметку.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"title": map[string]interface{}{
							"type":        "string",
							"description": "Название страницы в Notion (краткое и понятное)",
						},
					},
					"required": []string{"title"},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "search_notion",
				Description: "Ищет информацию в ранее сохранённых диалогах в Notion. Используется когда пользователь спрашивает про прошлые беседы или нужно найти ранее обсуждённые темы.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "Поисковый запрос для поиска в Notion",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "create_notion_page",
				Description: "Создаёт новую страницу в Notion с произвольным содержимым. Используется для создания заметок, планов, или структурированной информации.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"title": map[string]interface{}{
							"type":        "string",
							"description": "Название страницы",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "Содержимое страницы в формате Markdown",
						},
						"parent_page": map[string]interface{}{
							"type":        "string",
							"description": "Название родительской страницы (опционально)",
						},
					},
					"required": []string{"title", "content"},
				},
			},
		},
	}
}
