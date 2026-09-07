import assert from "node:assert/strict";
import test from "node:test";
import type { ChatInteractionIntent } from "../../models/chatInteraction";
import { chatInteractionService } from "./chatInteractionService.ts";

test("encodes question answers with the App Server answer envelope", () => {
  assert.deepEqual(
    chatInteractionService.encodeResponse("item/tool/requestUserInput", {
      kind: "answer_questions",
      answers: { framework: ["Preact"] },
    }),
    { result: { answers: { framework: { answers: ["Preact"] } } } }
  );
});

test("preserves current and legacy approval decisions", () => {
  assert.deepEqual(
    chatInteractionService.encodeResponse("item/commandExecution/requestApproval", {
      kind: "approve",
      scope: "once",
    }),
    { result: { decision: "accept" } }
  );
  assert.deepEqual(
    chatInteractionService.encodeResponse("item/fileChange/requestApproval", {
      kind: "approve",
      scope: "session",
    }),
    { result: { decision: "acceptForSession" } }
  );
  assert.deepEqual(
    chatInteractionService.encodeResponse("execCommandApproval", {
      kind: "approve",
      scope: "once",
    }),
    { result: { decision: "approved" } }
  );
  assert.deepEqual(
    chatInteractionService.encodeResponse("applyPatchApproval", {
      kind: "approve",
      scope: "session",
    }),
    { result: { decision: "approved_for_session" } }
  );
  assert.deepEqual(
    chatInteractionService.encodeResponse("execCommandApproval", {
      kind: "deny_approval",
    }),
    { result: { decision: { denied: { rejection: "Denied by user" } } } }
  );
  assert.deepEqual(
    chatInteractionService.encodeResponse("item/fileChange/requestApproval", {
      kind: "deny_approval",
    }),
    { result: { decision: "decline" } }
  );
  assert.deepEqual(
    chatInteractionService.encodeResponse("item/fileChange/requestApproval", {
      kind: "cancel_approval",
    }),
    { result: { decision: "cancel" } }
  );
  assert.equal(
    chatInteractionService.supportsApprovalCancellation("execCommandApproval"),
    false
  );
  assert.equal(
    chatInteractionService.supportsApprovalCancellation(
      "item/commandExecution/requestApproval"
    ),
    true
  );
});

test("preserves permission and elicitation response shapes", () => {
  const permissions = { filesystem: { read: true } };
  assert.deepEqual(
    chatInteractionService.encodeResponse("item/permissions/requestApproval", {
      kind: "grant_permissions",
      permissions,
      scope: "session",
    }),
    { result: { permissions, scope: "session" } }
  );
  assert.deepEqual(
    chatInteractionService.encodeResponse("item/permissions/requestApproval", {
      kind: "deny_permissions",
    }),
    { result: { permissions: {}, scope: "turn" } }
  );
  assert.deepEqual(
    chatInteractionService.encodeResponse("mcpServer/elicitation/request", {
      kind: "accept_elicitation",
      content: { choice: "yes" },
    }),
    { result: { action: "accept", content: { choice: "yes" } } }
  );
  assert.deepEqual(
    chatInteractionService.encodeResponse("mcpServer/elicitation/request", {
      kind: "decline_elicitation",
    }),
    { result: { action: "decline" } }
  );
  assert.deepEqual(
    chatInteractionService.encodeResponse("mcpServer/elicitation/request", {
      kind: "cancel_elicitation",
    }),
    { result: { action: "cancel" } }
  );
});

test("preserves generic results and unsupported JSON-RPC errors", () => {
  assert.deepEqual(
    chatInteractionService.encodeResponse("future/request", {
      kind: "submit_provider_result",
      result: { acknowledged: true },
    }),
    { result: { acknowledged: true } }
  );
  assert.deepEqual(
    chatInteractionService.encodeResponse("future/request", {
      kind: "decline_unsupported",
    }),
    {
      error: {
        code: -32601,
        message: "Unsupported provider request declined by user",
      },
    }
  );
});

test("preserves native review, dismissal, and delegated-task response payloads", () => {
  const cases = [
    ["kimi/approval", { decision: "accept", feedback: "", selected_label: undefined }],
    ["kimi/approval", { decision: "acceptForSession", feedback: "Keep tests", selected_label: undefined }],
    ["kimi/approval", { decision: "accept", feedback: "", selected_label: "Implement" }],
    ["kimi/approval", { decision: "decline", feedback: "Change scope", selected_label: "Revise" }],
    ["kimi/approval", { decision: "decline", feedback: "", selected_label: "Reject and Exit" }],
    ["kimi/question", { dismiss: true }],
    ["kimi/task", { action: "cancel" }],
    ["kimi/task", { action: "detach" }],
  ] as const;
  const intents: ChatInteractionIntent[] = [
    { kind: "review_approval", action: "allow_once", feedback: "" },
    { kind: "review_approval", action: "allow_session", feedback: "Keep tests" },
    { kind: "review_approval", action: "allow_once", feedback: "", optionLabel: "Implement" },
    { kind: "review_approval", action: "revise_plan", feedback: "Change scope" },
    { kind: "review_approval", action: "reject_plan", feedback: "" },
    { kind: "dismiss_questions" },
    { kind: "control_agent", action: "stop" },
    { kind: "control_agent", action: "run_in_background" },
  ];
  cases.forEach(([method, result], index) => {
    assert.deepEqual(chatInteractionService.encodeResponse(method, intents[index]), { result });
  });
  assert.deepEqual(chatInteractionService.encodeResponse("kimi/question", {
    kind: "answer_questions", answers: { choice: ["option-id", "Other answer"], free: ["Text"] },
  }), { result: { answers: { choice: { answers: ["option-id", "Other answer"] }, free: { answers: ["Text"] } } } });
});


test("delegated controls target only a live native task", () => {
  assert.deepEqual(chatInteractionService.delegatedTaskTarget({ stopInteractionId: "task:session:id" }, "running"),
    { id: "task:session:id", method: "kimi/task" });
  for (const status of ["completed", "failed", "interrupted", "cancelled", "canceled", "turnEnded"]) {
    assert.equal(chatInteractionService.delegatedTaskTarget({ stopInteractionId: "task:id" }, status), undefined);
  }
  assert.equal(chatInteractionService.delegatedTaskTarget({}, "running"), undefined);
  assert.equal(chatInteractionService.delegatedTaskTarget({ stopInteractionId: 1 }, "running"), undefined);
});
