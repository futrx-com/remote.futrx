// DiffView — structured commit diff: per-file cards, hunks, dual gutters.
import { DiffView } from "remote.futrx-web";

const commitDiff = `diff --git a/frontend/src/api/chatApi.ts b/frontend/src/api/chatApi.ts
--- a/frontend/src/api/chatApi.ts
+++ b/frontend/src/api/chatApi.ts
@@ -41,9 +41,12 @@ export async function sendMessage(chatId: string, body: SendBody) {
   const res = await fetch(\`/api/chats/\${chatId}/messages\`, {
     method: "POST",
     headers: { "Content-Type": "application/json" },
-    body: JSON.stringify(body),
+    body: JSON.stringify({ ...body, clientTs: Date.now() }),
   });
-  if (!res.ok) throw new Error("send failed");
+  if (!res.ok) {
+    throw new ApiError(res.status, await res.text());
+  }
   return res.json();
 }
@@ -78,7 +81,7 @@ export async function listChats(projectId: string) {
   const res = await fetch(\`/api/projects/\${projectId}/chats\`);
-  if (!res.ok) throw new Error("list failed");
+  if (!res.ok) throw new ApiError(res.status, await res.text());
   return res.json();
 }
diff --git a/frontend/src/api/ApiError.ts b/frontend/src/api/ApiError.ts
--- /dev/null
+++ b/frontend/src/api/ApiError.ts
@@ -0,0 +1,7 @@
+export class ApiError extends Error {
+  constructor(
+    public status: number,
+    message: string,
+  ) {
+    super(\`\${status}: \${message}\`);
+  }
+}`;

const removalDiff = `diff --git a/frontend/src/legacy/pollChats.ts b/frontend/src/legacy/pollChats.ts
--- a/frontend/src/legacy/pollChats.ts
+++ /dev/null
@@ -1,6 +0,0 @@
-// Deprecated: replaced by SSE stream in chatApi.ts
-export function pollChats(projectId: string, ms = 5000) {
-  return setInterval(() => {
-    void fetch(\`/api/projects/\${projectId}/chats\`);
-  }, ms);
-}
diff --git a/frontend/public/icons/agent.png b/frontend/public/icons/agent.png
Binary files a/frontend/public/icons/agent.png and b/frontend/public/icons/agent.png differ`;

const rawFallback = `Commit 9bb4ec2 touched only submodule pointers.
No textual diff is available for this revision.`;

export const CommitWithNewFile = () => (
  <div className="w-full max-w-2xl">
    <DiffView diff={commitDiff} />
  </div>
);

export const DeletedAndBinary = () => (
  <div className="w-full max-w-2xl">
    <DiffView diff={removalDiff} />
  </div>
);

export const RawFallback = () => (
  <div className="w-full max-w-2xl">
    <div className="rounded-lg border border-line bg-tint overflow-hidden">
      <DiffView diff={rawFallback} />
    </div>
  </div>
);
