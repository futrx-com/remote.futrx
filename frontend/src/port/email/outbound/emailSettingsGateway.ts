// Outbound port for the email settings controller: the HTTP boundary
// workflow it depends on, so it can be tested without global transport
// mocking. frontend/src/api/emailApi.ts is the production implementation.
import type {
  SaveSMTPSettingsRequest,
  SMTPSettings,
} from "../../../model/email/api/smtpSettings";

export interface EmailSettingsGateway {
  get(): Promise<SMTPSettings>;
  save(request: SaveSMTPSettingsRequest): Promise<SMTPSettings>;
  remove(): Promise<void>;
  sendTest(to: string): Promise<{ sent: boolean }>;
}
