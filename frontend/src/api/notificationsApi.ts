import { requestJson } from "./apiRequest";
import { API_ROUTES } from "../config/routes";
import type {
  NotificationSettings,
  NotificationTestResult,
  UpdateNotificationSettingsInput,
} from "../models/notifications";

export const notificationsApi = {
  get: () => requestJson<NotificationSettings>("GET", API_ROUTES.notifications.settings),
  save: (input: UpdateNotificationSettingsInput) =>
    requestJson<NotificationSettings>("PUT", API_ROUTES.notifications.settings, input),
  test: () =>
    requestJson<{ results: NotificationTestResult[] }>("POST", API_ROUTES.notifications.test),
};
