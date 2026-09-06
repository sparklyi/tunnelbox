import type { Operation, Service, ServiceState } from "./api/types";
import type { Locale, MessageKey, Translate } from "./i18n";

export function formatTime(value: string | undefined, locale: Locale) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return date.toLocaleString(locale, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
}

export function modeMeta(mode: Service["mode"], t: Translate) {
  switch (mode) {
    case "private":
      return { label: t("service.modePrivate"), tone: "private" };
    case "public":
      return { label: t("service.modePublic"), tone: "public" };
    default:
      return { label: t("service.modeQuick"), tone: "quick" };
  }
}

export function stateMeta(state: ServiceState, t: Translate) {
  switch (state) {
    case "active":
      return { label: t("service.stateActive"), tone: "active" };
    case "deploying":
      return { label: t("service.stateDeploying"), tone: "working" };
    case "stopping":
      return { label: t("service.stateStopping"), tone: "working" };
    case "stopped":
      return { label: t("service.stateStopped"), tone: "stopped" };
    case "error":
      return { label: t("service.stateError"), tone: "error" };
    default:
      return { label: t("service.stateDraft"), tone: "draft" };
  }
}

export function serviceHasRemoteResources(item: Service) {
  return Boolean(item.tunnel_id || item.private_route_id || item.dns_record_id || item.access_application_id || item.access_policy_id || item.public_url);
}

export function operationMeta(status: Operation["status"], t: Translate) {
  switch (status) {
    case "succeeded":
      return { label: t("operation.succeeded"), tone: "active" };
    case "failed":
      return { label: t("operation.failed"), tone: "error" };
    case "unknown":
      return { label: t("operation.unknown"), tone: "working" };
    case "running":
      return { label: t("operation.running"), tone: "working" };
    default:
      return { label: t("operation.pending"), tone: "draft" };
  }
}

export function operationLabel(kind: string, t: Translate) {
  if (kind === "deploy") return t("operation.deploy");
  if (kind === "stop") return t("operation.stop");
  if (kind === "delete") return t("operation.delete");
  return t("operation.recent");
}

const stepKeys: Partial<Record<MessageKey, true>> = {
  "step.origin_check": true,
  "step.zone_validation": true,
  "step.tunnel": true,
  "step.tunnel_route": true,
  "step.connector": true,
  "step.connector_health": true,
  "step.access_application": true,
  "step.access_policy": true,
  "step.access_policy_attach": true,
  "step.dns": true,
  "step.quick_tunnel": true,
  "step.connector_stop": true,
  "step.dns_delete": true,
  "step.access_policy_delete": true,
  "step.access_application_delete": true,
  "step.private_route_delete": true,
  "step.tunnel_delete": true,
  "step.connector_credentials_delete": true,
  "step.service_delete": true,
  "step.complete": true,
};

export function operationStepLabel(step: string | undefined, t: Translate) {
  if (!step) return t("operation.waiting");
  const key = `step.${step}` as MessageKey;
  return stepKeys[key] ? t(key) : step.replaceAll("_", " ");
}
