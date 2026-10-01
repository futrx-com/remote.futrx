export interface Question {
  question: string;
  header?: string;
  multiSelect?: boolean;
  options: Array<{ label: string; description?: string }>;
}

export interface AskUserQuestionInput {
  questions?: Question[];
}
