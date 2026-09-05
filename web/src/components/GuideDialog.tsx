import { motion } from "framer-motion";
import { ArrowRight, CircleHelp, ExternalLink, Settings2, X } from "lucide-react";

type GuideDialogProps = {
  onClose: () => void;
  onQuick: () => void;
  onConfigure: () => void;
};

export function GuideDialog({ onClose, onQuick, onConfigure }: GuideDialogProps) {
  return (
    <motion.div
      className="dialog-layer"
      role="presentation"
      tabIndex={-1}
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      exit={{ opacity: 0 }}
      onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}
      onKeyDown={(event) => { if (event.key === "Escape") onClose(); }}
    >
      <motion.aside
        className="dialog guide-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="guide-dialog-title"
        aria-describedby="guide-dialog-description"
        initial={{ x: 28, opacity: 0 }}
        animate={{ x: 0, opacity: 1 }}
        exit={{ x: 28, opacity: 0 }}
        transition={{ type: "spring", stiffness: 360, damping: 32 }}
      >
        <div className="dialog-header">
          <div className="guide-title-lockup">
            <div className="guide-icon" aria-hidden="true"><CircleHelp size={20} /></div>
            <div><p className="eyebrow">首次使用</p><h2 id="guide-dialog-title">快速开始</h2></div>
          </div>
          <button type="button" className="icon-button" onClick={onClose} aria-label="关闭使用指南" title="关闭使用指南"><X size={18} /></button>
        </div>
        <div className="guide-content">
          <p id="guide-dialog-description" className="guide-intro">先选适合你的出口方式。没有公网域名时也能马上开始，但不同方式的访问体验和权限边界不同。</p>
          <ol className="guide-steps">
            <li className="guide-step">
              <span className="guide-step-number" aria-hidden="true">1</span>
              <div>
                <h3>选择出口方式</h3>
                <p><strong>临时公开</strong>不需要域名、账号或 Token，会生成随机的 trycloudflare.com 地址；<strong>私网受控</strong>不需要公网域名，但访问者要加入同一个 Zero Trust 组织、安装并登录 WARP，还要让目标 IP 经过 Split Tunnel；<strong>自有域名</strong>适合普通浏览器访问和标准 Access 登录。</p>
                <div className="guide-links">
                  <a href="https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/do-more-with-tunnels/trycloudflare/" target="_blank" rel="noreferrer">Quick Tunnel 说明 <ExternalLink size={13} /></a>
                  <a href="https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/private-net/cloudflared/connect-cidr/" target="_blank" rel="noreferrer">私网路由说明 <ExternalLink size={13} /></a>
                  <a href="https://developers.cloudflare.com/cloudflare-one/team-and-resources/devices/cloudflare-one-client/configure/route-traffic/split-tunnels/" target="_blank" rel="noreferrer">Split Tunnels 配置 <ExternalLink size={13} /></a>
                </div>
              </div>
            </li>
            <li className="guide-step">
              <span className="guide-step-number" aria-hidden="true">2</span>
              <div>
                <h3>按需连接 Cloudflare</h3>
                <p>Quick 模式可以跳过连接设置。Private 只需 Account ID 和 Token；Public 还需要目标 Zone ID。Token 只会写入本机的受限 Secret 文件，不会显示在列表或响应中。</p>
                <div className="guide-note">
                  <strong>Public / Private 的最小权限</strong>
                  <ul className="guide-permissions">
                    <li>Account / Cloudflare Tunnel / Edit</li>
                    <li>Account / Access: Apps and Policies / Edit</li>
                    <li>Account / Zero Trust / Write</li>
                    <li>Public 额外需要 Zone / DNS / Edit、Zone / Zone / Read</li>
                  </ul>
                </div>
              </div>
            </li>
            <li className="guide-step">
              <span className="guide-step-number" aria-hidden="true">3</span>
              <div>
                <h3>填写 Origin</h3>
                <p>Origin URL 必须是运行 Connector 的机器可以访问的 HTTP/HTTPS 地址。Private 模式再填写同一台服务的私网 IP；Public 模式填写目标 Zone 下的主机名；Quick 模式不需要额外地址。</p>
              </div>
            </li>
            <li className="guide-step">
              <span className="guide-step-number" aria-hidden="true">4</span>
              <div>
                <h3>部署并验证</h3>
                <p>点击部署后等待操作面板完成。Quick 完成后直接打开随机地址；Private 先确认设备已 enrollment 且目标 IP 已走 WARP，再用 WARP 访问私网 IP；Public 最后创建 DNS，再用普通浏览器访问并测试 Access 登录。</p>
              </div>
            </li>
          </ol>
          <p className="guide-footnote">不确定字段含义时，可以查看仓库 README 的中英文完整说明。</p>
        </div>
        <div className="dialog-actions guide-actions">
          <button type="button" className="button button-secondary" onClick={onConfigure}><Settings2 size={16} />配置 Cloudflare</button>
          <button type="button" className="button button-primary" onClick={onQuick}><ArrowRight size={16} />直接创建 Quick</button>
        </div>
      </motion.aside>
    </motion.div>
  );
}
