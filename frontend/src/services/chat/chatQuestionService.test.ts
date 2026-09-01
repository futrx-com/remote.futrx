import assert from "node:assert/strict";
import test from "node:test";
import { chatQuestionService } from "./chatQuestionService.ts";

test("summarizes display text and structured answers keyed by question id", () => {
  const summary = chatQuestionService.summarizeAnswers([
    {
      id: "environment",
      header: "Environment",
      question: "Where should I deploy?",
      options: [{ label: "QA" }, { label: "Production" }],
    },
    {
      id: "checks",
      header: "Checks",
      question: "Which checks should run?",
      multiSelect: true,
      options: [{ label: "Tests" }, { label: "Lint" }],
    },
    {
      question: "Legacy question without an id?",
      options: [{ label: "Keep prompt fallback" }],
    },
  ], (index) => [
    ["QA"],
    ["Tests", "Lint"],
    ["Keep prompt fallback"],
  ][index]);

  assert.deepEqual(summary, {
    text: [
      "Q: Where should I deploy?\nA: QA",
      "Q: Which checks should run?\nA: Tests; Lint",
      "Q: Legacy question without an id?\nA: Keep prompt fallback",
    ].join("\n\n"),
    preview: "Environment: QA · Checks: Tests, Lint · Answer: Keep prompt fallback",
    answers: {
      environment: ["QA"],
      checks: ["Tests", "Lint"],
    },
    sensitive: false,
  });
});

test("validates every interactive question id and rejects duplicates", () => {
  assert.equal(chatQuestionService.hasValidIds([
    { id: "first", question: "First?", options: [] },
    { id: "second", question: "Second?", options: [] },
  ]), true);
  assert.equal(chatQuestionService.hasValidIds([
    { id: "first", question: "First?", options: [] },
    { question: "Missing?", options: [] },
  ]), false);
  assert.equal(chatQuestionService.hasValidIds([
    { id: "same", question: "First?", options: [] },
    { id: " same ", question: "Duplicate?", options: [] },
  ]), false);
});

test("preserves a Codex option with its additional native note", () => {
  const summary = chatQuestionService.summarizeAnswers([{
    id: "scope",
    header: "Scope",
    question: "Which scope should I use?",
    isOther: true,
    options: [{ label: "Backend" }, { label: "Frontend" }],
  }], () => ["Backend", "deployment details"]);

  assert.deepEqual(summary.answers, {
    scope: ["Backend", "deployment details"],
  });
  assert.equal(summary.preview, "Scope: Backend, deployment details");
});

test("derives a readable resolved preview from backend interaction output", () => {
  const questions = [
    {
      id: "environment",
      header: "Environment",
      question: "Where should I deploy?",
      options: [{ label: "QA" }],
    },
    {
      id: "checks",
      header: "Checks",
      question: "Which checks should run?",
      options: [{ label: "Tests" }],
    },
  ];

  assert.equal(chatQuestionService.resolvedPreview(
    questions,
    JSON.stringify({ Answers: { environment: ["QA"], checks: ["Tests", "Lint"] } }),
  ), "Environment: QA · Checks: Tests, Lint");
  assert.equal(chatQuestionService.resolvedPreview(
    questions,
    JSON.stringify({ answers: { environment: ["Production"] } }),
  ), "Environment: Production");
  assert.equal(chatQuestionService.resolvedPreview(questions, "not-json"), "Response received");
  assert.equal(
    chatQuestionService.resolvedPreview(
      questions,
      "No response before the agent continued",
    ),
    "No response before the agent continued",
  );
  assert.equal(
    chatQuestionService.resolvedPreview(questions, "Agent interaction cancelled"),
    "Agent interaction cancelled",
  );
});

test("never exposes secret answers in previews or resolved output", () => {
  const questions = [{
    id: "token",
    header: "Token",
    question: "What is the token?",
    isSecret: true,
    isOther: false,
    options: null,
  }];
  const summary = chatQuestionService.summarizeAnswers(questions, () => ["super-secret"]);

  assert.equal(summary.preview, "Token: Secret response received");
  assert.equal(summary.sensitive, true);
  assert.equal(summary.preview.includes("super-secret"), false);
  assert.equal(
    chatQuestionService.resolvedPreview(
      questions,
      JSON.stringify({ answers: { token: ["super-secret"] } }),
    ),
    "Secret response received",
  );
});
