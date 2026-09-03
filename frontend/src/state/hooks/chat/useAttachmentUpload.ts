import { useCallback, useEffect, useRef, useState } from "preact/hooks";
import type { Attachment } from "../../../models/upload";
import { startChatUpload } from "../../../api/uploadApi";
import type { UploadHandle } from "../../../types/uploadApi";
import { idService } from "../../../services/platform/idService.ts";
import { chatAttachmentService } from "../../../services/chat/chatAttachmentService.ts";

export function useAttachmentUpload(
  chatId: string,
  attachmentBasePath: string
) {
  const [attachments, setAttachments] = useState<Attachment[]>([]);
  const [uploading, setUploading] = useState(false);

  const attachmentBasePathRef = useRef(attachmentBasePath);
  // Outstanding tus handles, keyed by attachment id. Lets us abort on remove.
  const handlesRef = useRef<Map<string, UploadHandle>>(new Map());

  // Reaches state only through handlesRef and a setAttachments updater, so it
  // closes over nothing that can go stale — which is what makes [] honest here,
  // and what made the previous capture harmless rather than a bug.
  const clearAttachments = useCallback(() => {
    for (const handle of handlesRef.current.values()) void handle.abort();
    handlesRef.current.clear();
    setAttachments((prev) => {
      prev.forEach((attachment) => chatAttachmentService.revokeObjectUrl(attachment));
      return [];
    });
  }, []);

  useEffect(() => {
    clearAttachments();
  }, [chatId, clearAttachments]);

  useEffect(
    () => () => {
      clearAttachments();
    },
    [clearAttachments]
  );

  useEffect(() => {
    attachmentBasePathRef.current = attachmentBasePath;
  }, [attachmentBasePath]);

  const doUpload = useCallback(
    async (files: File[]) => {
      if (!files.length) return;
      // Pasted screenshots all arrive named "image.png", and the server stores
      // by filename — so a second paste would overwrite the first on disk and
      // both would resolve to the same prompt path. Give every upload a unique
      // storage name derived from its attachment id (which also disambiguates
      // the tus resume fingerprint), while keeping the original name as the
      // friendly label shown in the composer chip.
      const items = files.map((file) => {
        const id = idService.random();
        const uploadName = chatAttachmentService.uniqueUploadName(file.name, id);
        const uploadFile =
          uploadName === file.name
            ? file
            : new File([file], uploadName, {
                type: file.type,
                lastModified: file.lastModified,
              });
        return { id, displayName: file.name, uploadFile };
      });

      const queued: Attachment[] = items.map(({ id, displayName, uploadFile }) => ({
        id,
        name: displayName,
        size: uploadFile.size,
        serverPath: "",
        isImage: uploadFile.type.startsWith("image/"),
        objectUrl: uploadFile.type.startsWith("image/")
          ? URL.createObjectURL(uploadFile)
          : undefined,
        progress: 0,
      }));

      setAttachments((prev) => [...prev, ...queued]);
      setUploading(true);

      const finishedFlags: Promise<void>[] = [];
      for (let i = 0; i < items.length; i++) {
        const { uploadFile } = items[i];
        const att = queued[i];
        const done = new Promise<void>((resolve) => {
          const handle = startChatUpload(chatId, uploadFile, {
            onProgress(loaded, total) {
              const ratio = total > 0 ? loaded / total : 0;
              setAttachments((prev) =>
                prev.map((a) => (a.id === att.id ? { ...a, progress: ratio } : a))
              );
            },
            onSuccess() {
              handlesRef.current.delete(att.id);
              setAttachments((prev) =>
                prev.map((a) =>
                  a.id === att.id
                    ? {
                        ...a,
                        progress: 1,
                        serverPath: chatAttachmentService.absoluteUploadPath(
                          attachmentBasePathRef.current,
                          uploadFile.name
                        ),
                        error: undefined,
                      }
                    : a
                )
              );
              resolve();
            },
            onError(err) {
              handlesRef.current.delete(att.id);
              setAttachments((prev) =>
                prev.map((a) =>
                  a.id === att.id ? { ...a, error: err.message } : a
                )
              );
              resolve();
            },
          });
          handlesRef.current.set(att.id, handle);
        });
        finishedFlags.push(done);
      }

      await Promise.all(finishedFlags);
      setUploading(false);
    },
    [chatId]
  );

  const removeAttachment = useCallback((id: string) => {
    const handle = handlesRef.current.get(id);
    if (handle) {
      void handle.abort();
      handlesRef.current.delete(id);
    }
    setAttachments((prev) => {
      const target = prev.find((attachment) => attachment.id === id);
      if (target) chatAttachmentService.revokeObjectUrl(target);
      return prev.filter((attachment) => attachment.id !== id);
    });
  }, []);

  return {
    attachments,
    uploading,
    doUpload,
    removeAttachment,
    clearAttachments,
  };
}
