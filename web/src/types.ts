export type TaskState = 'accepted' | 'running' | 'completed' | 'failed' | 'canceled'

export type Task = {
  id: string
  kind: string
  title: string
  input: string
  state: TaskState
  summary: string
  result: string
  report_pending: boolean
  created_at: string
  updated_at: string
}

export type TaskEvent = {
  id: string
  task_id: string
  type: string
  message: string
  created_at: string
}

export type DashboardState = {
  tasks: Task[]
  events: TaskEvent[]
  claude_records: ClaudeRecord[]
  conversations: ConversationSnapshot[]
}

export type ConversationMessage = {
  role: string
  content: string
}

export type ConversationSnapshot = {
  id: string
  started_at: string
  last_active: string
  messages: ConversationMessage[]
}

export type ClaudeRecord = {
  task_id: string
  session_id: string
  prompt: string
  working_directory: string
  status: 'accepted' | 'running' | 'completed' | 'failed'
  last_summary: string
  last_assistant_text: string
  result: string
  error: string
  created_at: string
  updated_at: string
}
