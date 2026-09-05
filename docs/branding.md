# TunnelBox visual identity

The folded box represents a self-contained tool with an open passage through its
center. The cobalt right panel highlights the connection to the outside.

| Asset | Use |
| --- | --- |
| [Logo](../web/public/assets/logo.svg) | Console, sign-in screen, favicon, and square avatars |
| [Logo with wordmark](../web/public/assets/logo-wordmark.svg) | README and horizontal brand placement |

The SVG assets use graphite `#20242C`, cobalt `#4169E1`, and white `#FFFFFF`.
Both include a white background to keep the graphite mark legible on dark surfaces.
Preserve their aspect ratios and surrounding space when resizing.

The source assets live in `web/public/assets/`. Vite copies them into the build's
`assets/` directory, which the Go server already serves before login. Keep the
four panel paths consistent between the icon and wordmark when editing the mark.
