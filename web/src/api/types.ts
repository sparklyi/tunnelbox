export type AuthState = "loading" | "setup" | "login" | "authenticated";

export type ServiceState = "draft" | "deploying" | "stopping" | "active" | "stopped" | "error";
export type AllowType = "email" | "email_domain";
export type ExposureMode = "quick" | "private" | "public";

export type Service = {
  id: string;
  name: string;
  mode: ExposureMode;
  hostname?: string;
  origin_url: string;
  allow_type?: AllowType;
  allow_value?: string;
  state: ServiceState;
  tunnel_id?: string;
  private_route_id?: string;
  dns_record_id?: string;
  access_application_id?: string;
  access_policy_id?: string;
  public_url?: string;
  created_at: string;
  updated_at: string;
};

export type Operation = {
  operation_id: string;
  service_id: string;
  kind: string;
  status: "pending" | "running" | "succeeded" | "failed" | "unknown";
  current_step?: string;
  attempts: number;
  error_code?: string;
  error_message?: string;
  created_at: string;
  updated_at: string;
};

export type IntegrationStatus = {
  configured: boolean;
  account_id?: string;
  zone_id?: string;
  token_id?: string;
  token_state?: string;
  last_error?: string;
};

export type Zone = { id: string; name: string };

export type Connector = {
  service_id: string;
  mode?: string;
  running: boolean;
  healthy: boolean;
  url?: string;
  message?: string;
};

export type ServiceForm = {
  mode: ExposureMode;
  name: string;
  hostname: string;
  origin_url: string;
  allow_type: AllowType | "";
  allow_value: string;
};
