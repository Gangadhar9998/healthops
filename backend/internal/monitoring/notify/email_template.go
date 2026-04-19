package notify

import (
	"fmt"
	"strings"
	"time"
)

// buildHTMLEmail creates a professional, enterprise-grade HTML email for incident notifications.
func buildHTMLEmail(p NotificationPayload, dashboardURL string) string {
	isResolved := p.Status == "resolved"

	// Colors
	var accentColor, bgColor, statusLabel, statusIcon, severityBadge string
	switch {
	case isResolved:
		accentColor = "#10b981"
		bgColor = "#ecfdf5"
		statusLabel = "RESOLVED"
		statusIcon = "✅"
	case p.Severity == "critical":
		accentColor = "#ef4444"
		bgColor = "#fef2f2"
		statusLabel = "CRITICAL"
		statusIcon = "🔴"
	case p.Severity == "warning":
		accentColor = "#f59e0b"
		bgColor = "#fffbeb"
		statusLabel = "WARNING"
		statusIcon = "🟡"
	default:
		accentColor = "#3b82f6"
		bgColor = "#eff6ff"
		statusLabel = "INFO"
		statusIcon = "🔵"
	}
	_ = bgColor

	switch p.Severity {
	case "critical":
		severityBadge = fmt.Sprintf(`<span style="background:#fef2f2;color:#dc2626;padding:2px 10px;border-radius:12px;font-size:11px;font-weight:600;letter-spacing:0.5px;text-transform:uppercase;border:1px solid #fecaca">%s</span>`, p.Severity)
	case "warning":
		severityBadge = fmt.Sprintf(`<span style="background:#fffbeb;color:#d97706;padding:2px 10px;border-radius:12px;font-size:11px;font-weight:600;letter-spacing:0.5px;text-transform:uppercase;border:1px solid #fed7aa">%s</span>`, p.Severity)
	default:
		severityBadge = fmt.Sprintf(`<span style="background:#eff6ff;color:#2563eb;padding:2px 10px;border-radius:12px;font-size:11px;font-weight:600;letter-spacing:0.5px;text-transform:uppercase;border:1px solid #bfdbfe">%s</span>`, p.Severity)
	}

	resolvedRow := ""
	if p.ResolvedAt != "" {
		resolvedRow = fmt.Sprintf(`
					<tr>
						<td style="padding:8px 12px;font-size:13px;color:#6b7280;border-bottom:1px solid #f3f4f6;width:140px">Resolved At</td>
						<td style="padding:8px 12px;font-size:13px;color:#111827;border-bottom:1px solid #f3f4f6;font-weight:500">%s</td>
					</tr>`, p.ResolvedAt)
	}

	serverRow := ""
	if p.Server != "" {
		serverRow = fmt.Sprintf(`
					<tr>
						<td style="padding:8px 12px;font-size:13px;color:#6b7280;border-bottom:1px solid #f3f4f6;width:140px">Server</td>
						<td style="padding:8px 12px;font-size:13px;color:#111827;border-bottom:1px solid #f3f4f6;font-weight:500">%s</td>
					</tr>`, htmlEscape(p.Server))
	}

	dashboardButton := ""
	if dashboardURL != "" {
		incidentURL := fmt.Sprintf("%s/incidents/%s", strings.TrimRight(dashboardURL, "/"), p.IncidentID)
		dashboardButton = fmt.Sprintf(`
				<div style="text-align:center;margin:24px 0 8px">
					<a href="%s" style="display:inline-block;background:%s;color:#fff;padding:10px 28px;border-radius:6px;text-decoration:none;font-size:13px;font-weight:600;letter-spacing:0.3px">
						View Incident in Dashboard →
					</a>
				</div>`, incidentURL, accentColor)
	}

	year := time.Now().Year()

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width,initial-scale=1.0">
	<title>HealthOps Alert</title>
</head>
<body style="margin:0;padding:0;background:#f8fafc;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif">
	<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background:#f8fafc">
		<tr>
			<td align="center" style="padding:32px 16px">
				<table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width:600px;width:100%%">

					<!-- Header -->
					<tr>
						<td style="background:%s;padding:20px 24px;border-radius:12px 12px 0 0">
							<table role="presentation" width="100%%" cellpadding="0" cellspacing="0">
								<tr>
									<td>
										<span style="font-size:18px;font-weight:700;color:#fff;letter-spacing:-0.3px">
											%s HealthOps
										</span>
									</td>
									<td align="right">
										<span style="background:rgba(255,255,255,0.2);color:#fff;padding:4px 12px;border-radius:20px;font-size:11px;font-weight:600;letter-spacing:0.5px">
											%s
										</span>
									</td>
								</tr>
							</table>
						</td>
					</tr>

					<!-- Body -->
					<tr>
						<td style="background:#ffffff;padding:28px 24px;border-left:1px solid #e5e7eb;border-right:1px solid #e5e7eb">

							<!-- Title -->
							<h1 style="margin:0 0 6px;font-size:20px;font-weight:700;color:#111827;line-height:1.3">
								%s
							</h1>
							<p style="margin:0 0 20px;font-size:13px;color:#6b7280">
								Incident ID: <code style="background:#f3f4f6;padding:1px 6px;border-radius:4px;font-size:12px">%s</code>
							</p>

							<!-- Message -->
							<div style="background:#f9fafb;border:1px solid #e5e7eb;border-radius:8px;padding:14px 16px;margin-bottom:20px">
								<p style="margin:0;font-size:13px;color:#374151;line-height:1.6;word-break:break-word">
									%s
								</p>
							</div>

							<!-- Details Table -->
							<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="border:1px solid #e5e7eb;border-radius:8px;overflow:hidden">
								<tr>
									<td style="padding:8px 12px;font-size:13px;color:#6b7280;border-bottom:1px solid #f3f4f6;width:140px">Check</td>
									<td style="padding:8px 12px;font-size:13px;color:#111827;border-bottom:1px solid #f3f4f6;font-weight:600">%s</td>
								</tr>
								<tr>
									<td style="padding:8px 12px;font-size:13px;color:#6b7280;border-bottom:1px solid #f3f4f6">Type</td>
									<td style="padding:8px 12px;font-size:13px;color:#111827;border-bottom:1px solid #f3f4f6;font-weight:500">%s</td>
								</tr>
								<tr>
									<td style="padding:8px 12px;font-size:13px;color:#6b7280;border-bottom:1px solid #f3f4f6">Severity</td>
									<td style="padding:8px 12px;border-bottom:1px solid #f3f4f6">%s</td>
								</tr>%s
								<tr>
									<td style="padding:8px 12px;font-size:13px;color:#6b7280;border-bottom:1px solid #f3f4f6">Started</td>
									<td style="padding:8px 12px;font-size:13px;color:#111827;border-bottom:1px solid #f3f4f6;font-weight:500">%s</td>
								</tr>%s
								<tr>
									<td style="padding:8px 12px;font-size:13px;color:#6b7280">Status</td>
									<td style="padding:8px 12px;font-size:13px;font-weight:600;color:%s">%s</td>
								</tr>
							</table>

							%s
						</td>
					</tr>

					<!-- Footer -->
					<tr>
						<td style="background:#f9fafb;padding:16px 24px;border:1px solid #e5e7eb;border-top:none;border-radius:0 0 12px 12px">
							<table role="presentation" width="100%%" cellpadding="0" cellspacing="0">
								<tr>
									<td>
										<p style="margin:0;font-size:11px;color:#9ca3af;line-height:1.5">
											Sent by <strong style="color:#6b7280">HealthOps Monitoring</strong><br>
											This is an automated alert — do not reply to this email.
										</p>
									</td>
									<td align="right">
										<p style="margin:0;font-size:11px;color:#9ca3af">
											© %d HealthOps
										</p>
									</td>
								</tr>
							</table>
						</td>
					</tr>
				</table>
			</td>
		</tr>
	</table>
</body>
</html>`,
		accentColor,                    // header background
		statusIcon,                     // header icon
		statusLabel,                    // header badge
		htmlEscape(p.CheckName),        // title
		htmlEscape(p.IncidentID),       // incident ID
		htmlEscape(p.Message),          // message
		htmlEscape(p.CheckName),        // details: check name
		htmlEscape(p.CheckType),        // details: type
		severityBadge,                  // details: severity badge
		serverRow,                      // details: server (conditional)
		htmlEscape(p.StartedAt),        // details: started
		resolvedRow,                    // details: resolved (conditional)
		accentColor,                    // status color
		statusLabel,                    // status label
		dashboardButton,               // CTA button
		year,                           // footer year
	)
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}
