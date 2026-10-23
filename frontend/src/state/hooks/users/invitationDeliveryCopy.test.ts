import assert from "node:assert/strict";
import test from "node:test";
import { invitationDeliveryCopy } from "./invitationDeliveryCopy.ts";

const user = { email: "a@example.com", role: "member" as const, addedAt: 1 };
const issued = { method: "password" as const, temporaryPassword: "AAAAA-BBBBB-CCCCC-DDDDD", user };

test("sent: confirms the email and asks before showing the credentials", () => {
  assert.deepEqual(invitationDeliveryCopy.describe({ ...issued, delivery: "sent" }), {
    tone: "success",
    message: "Invitation emailed to a@example.com",
    credentialsVisible: false,
    revealQuestion: "View or download the credentials too?",
  });
});

test("not_requested: shows the credentials with nothing alarming", () => {
  const copy = invitationDeliveryCopy.describe({ ...issued, delivery: "not_requested" });
  assert.equal(copy.tone, "neutral");
  assert.equal(copy.credentialsVisible, true);
  assert.equal(copy.revealQuestion, undefined);
});

test("not_configured: warns that nothing was emailed", () => {
  assert.deepEqual(invitationDeliveryCopy.describe({ ...issued, delivery: "not_configured" }), {
    tone: "warning",
    message: "Not emailed: SMTP is not configured",
    credentialsVisible: true,
  });
});

test("failed: names the reason and says the account exists", () => {
  assert.deepEqual(
    invitationDeliveryCopy.describe({ ...issued, delivery: "failed", deliveryError: "Could not reach the SMTP server" }),
    {
      tone: "error",
      message: "The email could not be sent: Could not reach the SMTP server. The account was created.",
      credentialsVisible: true,
    },
  );
});

test("failed: a reason that already ends in a period is not doubled, and a missing one is omitted", () => {
  assert.equal(
    invitationDeliveryCopy.describe({ ...issued, delivery: "failed", deliveryError: "Rejected." }).message,
    "The email could not be sent: Rejected. The account was created.",
  );
  assert.equal(
    invitationDeliveryCopy.describe({ ...issued, delivery: "failed" }).message,
    "The email could not be sent. The account was created.",
  );
});
