// "" is a registered non-admin; their access comes from custom roles.
export type UserRole = "admin" | "";

export interface User {
  email: string;
  role: UserRole;
  addedAt: number;
  addedBy?: string;
}
