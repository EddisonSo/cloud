import type { LucideIcon } from "lucide-react";

// ── Compute ──────────────────────────────────────────────

export interface Container {
  id: string;
  name: string;
  status: string;
  hostname?: string;
  external_ip?: string;
  ssh_enabled?: boolean;
  https_enabled?: boolean;
  owner?: string;
  memory_mb?: number;
  storage_gb?: number;
}

export type ContainerAction = "starting" | "stopping" | "deleting";

export interface ContainerStatusUpdate {
  container_id: string;
  status: string;
  external_ip?: string;
}

export interface IngressRule {
  port: number;
  target_port: number;
}

export interface SshKey {
  id: string;
  name: string;
  public_key: string;
  fingerprint?: string;
}

export interface CreateContainerData {
  name: string;
  memory_mb: number;
  storage_gb: number;
  instance_type: string;
  ssh_key_ids: string[];
  enable_ssh: boolean;
  ingress_rules: IngressRule[];
  mount_paths: string[];
}

// ── Storage ──────────────────────────────────────────────

export interface Namespace {
  name: string;
  count: number;
  hidden: boolean;
  visibility: NamespaceVisibility;
}

export type NamespaceVisibility = 0 | 1 | 2; // 0=private, 1=unlisted, 2=public

export interface FileEntry {
  name: string;
  size: number;
  modified: number;
  namespace: string;
}

export interface UploadProgress {
  bytes: number;
  total: number;
  active: boolean;
}

export interface UploadResult {
  success: boolean;
  fileExists?: boolean;
  fileName?: string;
}

// ── Auth ─────────────────────────────────────────────────

export interface User {
  user_id: string;
  username: string;
  display_name?: string;
  is_admin?: boolean;
}

export interface JwtPayload {
  username: string;
  user_id?: string;
  display_name?: string;
  is_admin?: boolean;
  exp?: number;
}

// ── Health ────────────────────────────────────────────────

export interface ClusterNode {
  name: string;
  conditions?: NodeCondition[];
  cpu_usage?: string;
  cpu_capacity?: string;
  cpu_percent?: number;
  memory_usage?: string;
  memory_capacity?: string;
  memory_percent?: number;
  disk_usage?: number;
  disk_capacity?: number;
  disk_percent?: number;
}

export interface NodeCondition {
  type: string;
  status: string;
}

export interface HealthState {
  cluster_ok: boolean;
  nodes: ClusterNode[];
}

export interface Pod {
  name: string;
  namespace?: string;
  node?: string;
  cpu_usage?: number;
  cpu_capacity?: number;
  memory_usage?: number;
  memory_capacity?: number;
  disk_usage?: number;
  disk_capacity?: number;
}

export interface PodMetrics {
  pods: Pod[];
}

// ── Observability ────────────────────────────────────────

export interface LogEntry {
  level: LogLevel;
  source: string;
  timestamp: string;
  message: string;
}

export type LogLevel = 0 | 1 | 2 | 3; // 0=DEBUG, 1=INFO, 2=WARN, 3=ERROR

// ── Notifications ────────────────────────────────────────

export interface Notification {
  id: string;
  title?: string;
  message: string;
  category?: string;
  link?: string;
  read: boolean;
  created_at?: string;
}

// ── Service Accounts ─────────────────────────────────────

export interface ServiceAccount {
  id: string;
  name: string;
  scopes?: string[];
  token_count?: number;
  created_at?: string;
}

export interface ApiToken {
  id: string;
  name: string;
  token?: string;
  service_account_id?: string;
  service_account_name?: string;
  expires_at?: string;
  created_at?: string;
}

// ── Navigation ───────────────────────────────────────────

export interface NavSubItem {
  id: string;
  label: string;
  icon: LucideIcon;
  path: string;
}

export interface NavItem {
  id: string;
  label: string;
  icon: LucideIcon;
  path: string;
  subItems?: NavSubItem[];
}

export interface TabCopy {
  eyebrow: string;
  title: string;
  lead: string;
}

// ── WebSocket Messages ───────────────────────────────────

export type ComputeWsMessage =
  | { type: "containers"; data: Container[] }
  | { type: "container_status"; data: ContainerStatusUpdate };

export type HealthSseMessage =
  | { type: "cluster"; payload: { nodes: ClusterNode[] } }
  | { type: "pods"; payload: PodMetrics };

// ── Admin ────────────────────────────────────────────────

export interface AdminUser {
  user_id: string;
  username: string;
  display_name?: string;
}

export interface AdminSession {
  user_id: string;
  username?: string;
  display_name?: string;
  ip_address?: string;
  created_at?: number;
}

export interface AdminNamespace extends Namespace {
  owner_id?: number | null;
}

// ── Settings ─────────────────────────────────────────────

export interface SecurityKey {
  id: string;
  name: string;
  authenticator_type: string;
  created_at: number;
}

export interface UserSession {
  id: number;
  ip_address: string;
  created_at: number;
  is_current: boolean;
}

// ── Notification Mute ────────────────────────────────────

export interface NotificationMute {
  category: string;
  scope: string;
}
