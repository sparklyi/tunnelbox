import { useEffect } from "react";
import type { Dispatch, SetStateAction } from "react";
import { request } from "../api/client";
import type { Operation } from "../api/types";

export function useOperationPolling(
  operation: Operation | null,
  setOperation: Dispatch<SetStateAction<Operation | null>>,
  loadData: (showRefresh?: boolean) => Promise<boolean>,
  setError: Dispatch<SetStateAction<string>>,
) {
  useEffect(() => {
    if (!operation || ["succeeded", "failed", "unknown"].includes(operation.status)) return;
    let stopped = false;
    const timer = window.setInterval(async () => {
      try {
        const next = await request<Operation>(`/api/v1/operations/${operation.operation_id}`);
        if (!stopped) {
          setOperation(next);
          if (["succeeded", "failed", "unknown"].includes(next.status)) {
            await loadData(true);
          }
        }
      } catch (caught) {
        if (!stopped) setError(caught instanceof Error ? caught.message : "无法读取操作进度");
      }
    }, 1000);
    return () => {
      stopped = true;
      window.clearInterval(timer);
    };
  }, [loadData, operation, setError, setOperation]);
}
