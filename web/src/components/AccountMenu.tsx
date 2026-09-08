import { KeyRound, LogOut, Settings2 } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";
import { useLocale } from "../i18n";

type AccountMenuProps = {
  onChangePassword: () => void;
  onLogout: () => void;
};

export function AccountMenu({ onChangePassword, onLogout }: AccountMenuProps) {
  const { t } = useLocale();
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    menuRef.current?.querySelector<HTMLButtonElement>("[role='menuitem']")?.focus();

    function closeOnOutsidePointer(event: globalThis.PointerEvent) {
      const target = event.target;
      if (!(target instanceof Node) || (!menuRef.current?.contains(target) && !triggerRef.current?.contains(target))) {
        setOpen(false);
      }
    }
    function closeOnOutsideFocus(event: globalThis.FocusEvent) {
      const target = event.target;
      if (!(target instanceof Node) || (!menuRef.current?.contains(target) && !triggerRef.current?.contains(target))) {
        setOpen(false);
      }
    }
    function closeOnEscape(event: globalThis.KeyboardEvent) {
      if (event.key !== "Escape") return;
      event.preventDefault();
      setOpen(false);
      triggerRef.current?.focus();
    }

    document.addEventListener("pointerdown", closeOnOutsidePointer);
    document.addEventListener("focusin", closeOnOutsideFocus);
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("pointerdown", closeOnOutsidePointer);
      document.removeEventListener("focusin", closeOnOutsideFocus);
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [open]);

  function choose(action: () => void) {
    setOpen(false);
    triggerRef.current?.focus();
    action();
  }

  function moveFocus(event: ReactKeyboardEvent<HTMLDivElement>) {
    if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
    const items = Array.from(event.currentTarget.querySelectorAll<HTMLButtonElement>("[role='menuitem']"));
    if (items.length === 0) return;
    event.preventDefault();
    const current = items.indexOf(document.activeElement as HTMLButtonElement);
    if (event.key === "Home") items[0].focus();
    else if (event.key === "End") items[items.length - 1].focus();
    else if (event.key === "ArrowDown") items[(current + 1) % items.length].focus();
    else items[(current - 1 + items.length) % items.length].focus();
  }

  return (
    <div className="account-menu">
      <button
        ref={triggerRef}
        type="button"
        className="icon-button settings-button"
        onClick={() => setOpen((current) => !current)}
        aria-label={t("app.settings")}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls="account-menu"
        title={t("app.settings")}
      >
        <Settings2 size={17} />
      </button>
      {open && (
        <div ref={menuRef} id="account-menu" className="account-menu-popover" role="menu" aria-label={t("app.settings")} onKeyDown={moveFocus}>
          <button type="button" className="account-menu-item" role="menuitem" onClick={() => choose(onChangePassword)}>
            <KeyRound size={16} />
            <span>{t("app.changePassword")}</span>
          </button>
          <button type="button" className="account-menu-item danger" role="menuitem" onClick={() => choose(onLogout)}>
            <LogOut size={16} />
            <span>{t("app.logout")}</span>
          </button>
        </div>
      )}
    </div>
  );
}
