import type { Operation, Service, ServiceState } from "./api/types";

export function formatTime(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return date.toLocaleString("zh-CN", { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
}

export function modeMeta(mode: Service["mode"]) {
  switch (mode) {
    case "private":
      return { label: "私网受控", tone: "private" };
    case "public":
      return { label: "自有域名", tone: "public" };
    default:
      return { label: "临时公开", tone: "quick" };
  }
}

export function stateMeta(state: ServiceState) {
  switch (state) {
    case "active":
      return { label: "运行中", tone: "active" };
    case "deploying":
      return { label: "部署中", tone: "working" };
    case "stopping":
      return { label: "停止中", tone: "working" };
    case "stopped":
      return { label: "已停止", tone: "stopped" };
    case "error":
      return { label: "异常", tone: "error" };
    default:
      return { label: "草稿", tone: "draft" };
  }
}

export function serviceHasRemoteResources(item: Service) {
  return Boolean(item.tunnel_id || item.private_route_id || item.dns_record_id || item.access_application_id || item.access_policy_id || item.public_url);
}

export function operationMeta(status: Operation["status"]) {
  switch (status) {
    case "succeeded":
      return { label: "已完成", tone: "active" };
    case "failed":
      return { label: "失败", tone: "error" };
    case "unknown":
      return { label: "待确认", tone: "working" };
    case "running":
      return { label: "执行中", tone: "working" };
    default:
      return { label: "排队中", tone: "draft" };
  }
}

export function operationLabel(kind: string) {
  if (kind === "deploy") return "部署操作";
  if (kind === "stop") return "停止操作";
  if (kind === "delete") return "删除操作";
  return "最近操作";
}
