package main

import (
	"encoding/json"
	"strings"
	"time"
)

// Prompts ported verbatim from MyTasks app/src/main/res/values-ru/strings.xml
// (prompt_add_task_date / prompt_add_task_start / prompt_add_task_end /
// task_search_prompt_static) so backend mode behaves exactly like direct mode.

const promptAddTaskDate = "Сейчас - "

const promptAddTaskStart = ". Сформируй JSON‑ответ на основе приведённого текстового описания задачи и времени запроса - "

const promptAddTaskEnd = `. В JSON добавь поля: title - краткое описание задачи, не более 5 слов, description - полное описание задачи, type - тип задачи (выбери одно из DAILY — для задач сроком менее 1 недели, MEDIUM — для задач сроком менее 1 месяца, LARGE - для задач сроком менее 1 года), date - дата и время окончания срока задачи в формате "yyyy-MM-dd HH:mm:ss". Если срок окончания задачи не указан, считай срок до конца сегодняшнего дня. В ответе выведи только сформированный JSON`

const taskSearchPromptStatic = `Ты — ассистент по поиску задач в базе данных.
На вход ты получаешь: (1) текстовый запрос пользователя, (2) список задач в формате JSON, где каждая задача соответствует модели Task.

Твоя задача: найти наиболее релевантные задачи и вернуть:
1) текст запроса с исправленными грамматическими ошибками
2) подробный ответ пользователю на текстовый запрос. Ответ на русском языке. В ответе перечилси только названия всех релевантных задач. Ответ должен быть оптимизирован для проговаривания голосом. В ответе указывай семанитику полей (пример - говори не daily, а срочные задачи), раздели и отдельно проговори задачи по типу - срочные, среднесрочные, долгосрочные
3) список ids подходящих задач.

ВАЖНО:
- Отвечай СТРОГО в формате JSON, без Markdown и без любого текста вне JSON.
- Используй только данные из списка задач. Ничего не выдумывай.
- Если подходящих задач нет — верни пустой массив ids: [].
- Выбирай задачи по смыслу и фильтрам из запроса. Учитывай title, description, created, deadLine, а также поля type/status.

Семантика полей Task:
- type: "daily" / "medium" / "large" (в запросе: "срочные", "ежедневные", "на сегодня" → daily; "среднесрочные" → medium; "долгосрочные" → large)
- status:
  "started" (в запросе: "начатые", "в работе", "started")
  "waiting" (в запросе: "ожидающие", "к выполнению", "waiting")
  "paused"  (в запросе: "приостановленные", "на паузе", "paused")
  "stopped" (в запросе: "завершенные", "выполненные", "done", "completed", "stopped")

Правила поиска и отбора:
1) Если запрос явно содержит фильтры по type/status — сначала отфильтруй по ним.
2) Если запрос содержит ключевые слова/фразы по теме — ищи по title и description (частичные совпадения и смысловые совпадения).
3) Если в запросе есть условия по дедлайну/дате (например "сегодня", "завтра", "до пятницы", "просроченные") — используй deadLine (если дан) как основу сравнения.
4) Если фильтры не указаны, выбирай задачи по максимальной релевантности к тексту.
5) Если запрос просит "все" в рамках фильтра — верни все подходящие id.
6) Не включай задачи, которые не соответствуют явным ограничениям пользователя.

Формат ответа (строго):
{
        "request": string,
        "answer": string,
        "ids": string[]
}`

// buildAddTaskPrompt mirrors createNewTaskRequest() in the app:
// date const + timestamp + start const + text + end const.
func buildAddTaskPrompt(text string, now time.Time) string {
	return promptAddTaskDate +
		now.Format("2006-01-02 15:04:05") +
		promptAddTaskStart +
		text +
		promptAddTaskEnd
}

// buildSearchPrompt mirrors createSearchRequest() in the app.
func buildSearchPrompt(request string, tasks json.RawMessage) string {
	return strings.TrimSpace(taskSearchPromptStatic) +
		"\n\n" +
		"Запрос пользователя:\n" +
		strings.TrimSpace(request) +
		"\n\n" +
		"Список задач (JSON, массив Task):\n" +
		string(tasks)
}

// stripFences is the counterpart of the app's extractJsonFromContent():
// remove markdown fences and trim whitespace.
func stripFences(content string) string {
	return strings.TrimSpace(
		strings.ReplaceAll(
			strings.ReplaceAll(content, "```json", ""),
			"```", "",
		),
	)
}

// extractJSONObject is a safety net: the substring from the first '{' to the last '}'.
func extractJSONObject(content string) string {
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return content
	}
	return content[start : end+1]
}
