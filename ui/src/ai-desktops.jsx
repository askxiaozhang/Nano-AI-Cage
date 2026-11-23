import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  AppWindow,
  ChevronDown,
  ChevronRight,
  Download,
  ExternalLink,
  Globe2,
  Monitor,
  Pin,
  PinOff,
  Plus,
  Power,
  RefreshCw,
  Trash2,
} from "lucide-react";
import { api } from "./api";
import { DesktopToolbar, useDesktopZoom } from "./claude-desktop";
import { SafetyMessage } from "./safety-message";
import { RemoteView, Modal } from "./views";
import { useWindowVisibility } from "./window-visibility";
import { useTranslation } from "./interface-language.mjs";
import "./ai-desktops.css";

export function useAIDesktops(running, jobID) {
  const [catalog, setCatalog] = useState({ arch: "", vm_running: false, apps: [] });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const requestRef = useRef(null);

  const refresh = useCallback(async () => {
    const req = (requestRef.current ??= api("/ai-desktops"));
    setLoading(true);
    try {
      const next = await req;
      setCatalog(next);
      setError("");
      return next;
    } catch (e) {
      setError(e.message);
      return null;
    } finally {
      if (requestRef.current === req) requestRef.current = null;
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [running, jobID, refresh]);

  const pin = useCallback(async (id, pinned) => {
    await api(`/ai-desktops/${id}/pin`, { pinned });
    await refresh();
  }, [refresh]);

  const addCustom = useCallback(async (payload) => {
    const created = await api("/ai-desktops/custom", payload);
    await refresh();
    return created;
  }, [refresh]);

  const deleteCustom = useCallback(async (id) => {
    await api(`/ai-desktops/custom/${id}`, undefined, "DELETE");
    await refresh();
  }, [refresh]);

  const sidebarApps = (catalog.apps || []).filter(
    (app) => app.installed || app.running || app.pinned,
  );

  return {
    arch: catalog.arch || "",
    apps: catalog.apps || [],
    sidebarApps,
    loading,
    error,
    refresh,
    pin,
    addCustom,
    deleteCustom,
  };
}

export function AIDesktopSidebarList({
  model,
  disabled,
  hubOpen,
  activeAppId,
  onOpenHub,
  onSelectApp,
  onStopApp,
  googleAccounts,
  activeGoogleSession,
  onSelectGoogleSession,
  onStopGoogleSession,
  onAddGoogleAccount,
}) {
  const tr = useTranslation();
  const [expanded, setExpanded] = useState(true);
  const items = model?.sidebarApps || [];
  const gAccounts = googleAccounts?.accounts || [];

  return (
    <div className="ai-desktops-nav">
      <div className="ai-desktops-nav-row">
        <button
          className={`sidebar-link ${hubOpen ? "selected" : ""}`}
          aria-current={hubOpen ? "page" : undefined}
          onClick={onOpenHub}
        >
          <AppWindow size={17} />
          {tr("AI Desktops")}
        </button>
        {items.length > 0 && (
          <button
            className="icon-button"
            aria-label={expanded ? tr("收起 AI Desktop 列表") : tr("展开 AI Desktop 列表")}
            aria-expanded={expanded}
            onClick={() => setExpanded((value) => !value)}
          >
            {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          </button>
        )}
      </div>
      {expanded && items.length > 0 && (
        <div className="ai-desktops-sidebar-list">
          {items.map((app) => (
            <React.Fragment key={app.id}>
              <div className="ai-desktops-sidebar-row">
                <button
                  className={`sidebar-link ${activeAppId === app.id ? "selected" : ""}`}
                  disabled={disabled}
                  title={app.name + (app.running ? ` · ${tr("运行中")}` : app.installed ? ` · ${tr("已安装")}` : ` · ${tr("未安装")}`)}
                  onClick={() => onSelectApp(app.id)}
                >
                  <Monitor size={15} />
                  <span className="ai-desktops-sidebar-label">{app.name}</span>
                  {app.running && <span className="ai-desktops-running-dot" aria-label={tr("运行中")}>●</span>}
                </button>
                {app.running && onStopApp && (
                  <button
                    className="icon-button"
                    disabled={disabled}
                    aria-label={`${tr("关闭")} ${app.name}`}
                    title={`${tr("关闭")} ${app.name}`}
                    onClick={() => onStopApp(app)}
                  >
                    <Power size={13} />
                  </button>
                )}
              </div>
              {app.id === "antigravity" && app.installed && googleAccounts?.supported && (
                <div className="google-sidebar-sublist">
                  {gAccounts.map((acc) => {
                    const caption = acc.label || acc.email || acc.detected_email || `Google #${acc.number}`;
                    const agSelected = activeGoogleSession?.accountId === acc.id && activeGoogleSession?.mode === "antigravity";
                    const brSelected = activeGoogleSession?.accountId === acc.id && activeGoogleSession?.mode === "browser";
                    return (
                      <div className="google-sidebar-subrow" key={acc.id}>
                        <button
                          className={`sidebar-link ${agSelected ? "selected" : ""}`}
                          disabled={disabled}
                          title={`${caption}${acc.antigravity_running ? " · " + tr("运行中") : ""}`}
                          onClick={() => onSelectGoogleSession?.(acc, "antigravity")}
                        >
                          <Monitor size={13} />
                          <span className="ai-desktops-sidebar-label">{caption}</span>
                          {acc.antigravity_running && (
                            <span className="ai-desktops-running-dot" aria-label={tr("运行中")}>●</span>
                          )}
                        </button>
                        <button
                          className={`icon-button ${brSelected ? "selected" : ""}`}
                          disabled={disabled}
                          title={`${caption} · ${tr("独立浏览器")}`}
                          aria-label={`${caption} ${tr("独立浏览器")}`}
                          onClick={() => onSelectGoogleSession?.(acc, "browser")}
                        >
                          <Globe2 size={13} />
                        </button>
                        {acc.antigravity_running && onStopGoogleSession && (
                          <button
                            className="icon-button"
                            disabled={disabled}
                            title={`${tr("关闭")} ${caption}`}
                            aria-label={`${tr("关闭")} ${caption}`}
                            onClick={() => onStopGoogleSession(acc, "antigravity")}
                          >
                            <Power size={12} />
                          </button>
                        )}
                      </div>
                    );
                  })}
                  {onAddGoogleAccount && (
                    <button
                      className="sidebar-link"
                      style={{ fontSize: "12px", opacity: 0.8 }}
                      onClick={onAddGoogleAccount}
                    >
                      <Plus size={13} />
                      <span>{tr("管理 / 新建 Google 窗口")}</span>
                    </button>
                  )}
                </div>
              )}
            </React.Fragment>
          ))}
        </div>
      )}
    </div>
  );
}

export function AIDesktopHub({
  model,
  status,
  onSelectApp,
  onConfirm,
  onStatus,
  onDetails,
}) {
  const tr = useTranslation();
  const [adding, setAdding] = useState(false);
  const [form, setForm] = useState({ name: "", source: "", description: "", exec_command: "" });
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState("");
  const busy = Boolean(status?.job?.running);
  const running = Boolean(status?.vm_running);

  const runAction = async (app, action) => {
    try {
      await api(`/ai-desktops/${app.id}/${action}`, {});
      if (action === "install" || action === "update") {
        onStatus(`${tr("开始下载并安装")} ${app.name}…`);
        onSelectApp(app.id);
      } else {
        onStatus(`${app.name}: ${action}`);
      }
      void model.refresh();
    } catch (e) {
      onStatus(e.message, "error");
    }
  };

  const handleAddCustom = async (event) => {
    event.preventDefault();
    setSaving(true);
    setFormError("");
    try {
      const created = await model.addCustom(form);
      setAdding(false);
      setForm({ name: "", source: "", description: "", exec_command: "" });
      onStatus(`${tr("已添加")} ${created.name}`);
    } catch (e) {
      setFormError(e.message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <section className="ai-desktops-hub">
      <div className="ai-desktops-hub-header">
        <div>
          <p>
            {tr("在 Cage 隔离环境中下载并运行各类 AI Desktop 应用，已安装或固定的应用会显示在左侧边栏，点击即可像 Claude Desktop 一样直接使用。")}
            {model.arch && ` (${tr("当前客机架构")}: ${model.arch})`}
          </p>
        </div>
        <div className="ai-desktops-hub-actions">
          <button className="quiet-button" disabled={model.loading} onClick={() => model.refresh()}>
            <RefreshCw size={15} className={model.loading ? "spin" : undefined} />
            {tr("刷新列表")}
          </button>
          <button className="primary" onClick={() => setAdding(true)}>
            <Plus size={15} />
            {tr("添加自定义应用")}
          </button>
        </div>
      </div>

      {!running && (
        <div className="connection-banner" role="status">
          {tr("请先启动 Cage 环境，然后再下载或运行 AI Desktop 应用。")}
        </div>
      )}

      {busy && status.job?.kind?.startsWith("ai-desktop-") && (
        <div className="desktop-progress" role="status">
          <RefreshCw size={16} className="spin" />
          <span>{status.job.message}</span>
          <button className="text-button" onClick={onDetails}>
            {tr("查看进度")}
          </button>
        </div>
      )}

      {model.error && (
        <div className="connection-banner" role="alert">
          <SafetyMessage message={model.error} />
        </div>
      )}

      <div className="ai-desktops-grid">
        {model.apps.map((app) => (
          <article className="ai-desktop-card" key={app.id}>
            <div className="ai-desktop-card-head">
              <div className="ai-desktop-card-title">
                <Monitor size={18} />
                <strong>{app.name}</strong>
              </div>
              <div style={{ display: "flex", gap: 4 }}>
                <button
                  className="icon-button"
                  title={app.pinned ? tr("从侧栏取消固定") : tr("固定到左侧边栏")}
                  aria-label={app.pinned ? tr("从侧栏取消固定") : tr("固定到左侧边栏")}
                  onClick={() => model.pin(app.id, !app.pinned).catch((e) => onStatus(e.message, "error"))}
                >
                  {app.pinned ? <PinOff size={15} /> : <Pin size={15} />}
                </button>
                {app.homepage && (
                  <a
                    className="icon-button"
                    href={app.homepage}
                    target="_blank"
                    rel="noreferrer"
                    title={tr("打开官网或发布页")}
                    aria-label={tr("打开官网或发布页")}
                  >
                    <ExternalLink size={15} />
                  </a>
                )}
              </div>
            </div>

            <p className="ai-desktop-card-desc">{app.description}</p>

            <div className="ai-desktop-card-meta">
              {app.running && <span className="ai-desktop-badge running">● {tr("运行中")}</span>}
              {app.installed && (
                <span className="ai-desktop-badge installed">
                  {tr("已安装")}{app.version ? ` v${app.version}` : ""}
                </span>
              )}
              {!app.arch_supported && (
                <span className="ai-desktop-badge warn">
                  {tr("仅支持")} {(app.supported_arches || []).join("/")}
                </span>
              )}
              {app.custom && <span className="ai-desktop-badge">{tr("自定义")}</span>}
            </div>

            <div className="ai-desktop-card-actions">
              {app.installed ? (
                <>
                  <button
                    className="primary"
                    disabled={!running || busy}
                    onClick={() => onSelectApp(app.id)}
                  >
                    <Monitor size={15} />
                    {app.running ? tr("切换到画面") : tr("打开应用")}
                  </button>
                  <button
                    className="quiet-button"
                    disabled={!running || busy || !app.arch_supported}
                    onClick={() => runAction(app, "update")}
                  >
                    {tr("更新")}
                  </button>
                  <button
                    className="quiet-button"
                    disabled={!running || busy}
                    onClick={async () => {
                      if (
                        await onConfirm({
                          title: `卸载 ${app.name}？`,
                          body: "卸载会停止并移除应用安装包，用户配置目录通常保留。",
                          confirm: "卸载应用",
                          danger: true,
                        })
                      ) {
                        void runAction(app, "uninstall");
                      }
                    }}
                  >
                    <Trash2 size={14} />
                  </button>
                </>
              ) : (
                <button
                  className="primary"
                  disabled={!running || busy || !app.arch_supported}
                  onClick={() => runAction(app, "install")}
                >
                  <Download size={15} />
                  {tr("下载并安装")}
                </button>
              )}
              {app.custom && !app.installed && (
                <button
                  className="quiet-button"
                  disabled={busy}
                  onClick={async () => {
                    if (
                      await onConfirm({
                        title: `移除自定义应用 ${app.name}？`,
                        body: "将从 AI Desktops 列表中移除此条目。",
                        confirm: "移除",
                        danger: true,
                      })
                    ) {
                      model.deleteCustom(app.id).catch((e) => onStatus(e.message, "error"));
                    }
                  }}
                >
                  <Trash2 size={14} />
                  {tr("移除")}
                </button>
              )}
            </div>
          </article>
        ))}
      </div>

      {adding && (
        <Modal
          title={tr("添加自定义 AI Desktop")}
          onClose={() => !saving && setAdding(false)}
          footer={
            <>
              <button disabled={saving} onClick={() => setAdding(false)}>{tr("取消")}</button>
              <button className="primary" form="custom-ai-desktop-form" type="submit" disabled={saving || !form.name.trim() || !form.source.trim()}>
                {saving ? tr("保存中…") : tr("添加应用")}
              </button>
            </>
          }
        >
          <form id="custom-ai-desktop-form" className="modal-body ai-desktop-custom-form" onSubmit={handleAddCustom}>
            <label>
              <span>{tr("应用名称")}</span>
              <input
                required
                maxLength={60}
                placeholder="例如：Jan / LM Studio / AnythingLLM"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
              />
            </label>
            <label>
              <span>{tr("下载源（HTTPS .deb / .AppImage 链接，或 GitHub owner/repo）")}</span>
              <input
                required
                placeholder="https://...deb 或 https://...AppImage 或 janhq/jan"
                value={form.source}
                onChange={(e) => setForm({ ...form, source: e.target.value })}
              />
            </label>
            <label>
              <span>{tr("简介（可选）")}</span>
              <input
                maxLength={140}
                placeholder="简要描述应用用途"
                value={form.description}
                onChange={(e) => setForm({ ...form, description: e.target.value })}
              />
            </label>
            <label>
              <span>{tr("启动命令（可选，留空自动识别）")}</span>
              <input
                maxLength={120}
                placeholder="留空时自动从 .deb 桌面入口或 AppImage AppRun 识别"
                value={form.exec_command}
                onChange={(e) => setForm({ ...form, exec_command: e.target.value })}
              />
            </label>
            <p className="ai-desktop-custom-hint">
              {tr("提示：添加后会自动固定到左侧边栏，点击即可在 Cage 内下载安装并运行。")}
            </p>
            {formError && <div className="error-message" role="alert">{formError}</div>}
          </form>
        </Modal>
      )}
    </section>
  );
}

export function AIDesktopView({
  appId,
  initialApp,
  status,
  visible,
  remoteRef,
  onStatus,
  onFallback,
  onConfirm,
  onDetails,
  onExpand,
  sidebarCollapsed,
  onCatalogRefresh,
}) {
  const tr = useTranslation();
  const [zoom, setZoom] = useDesktopZoom();
  const windowVisible = useWindowVisibility();
  const screenVisible = visible && windowVisible;
  const [app, setApp] = useState(initialApp || null);
  const [error, setError] = useState("");
  const [statusError, setStatusError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [jobID, setJobID] = useState("");
  const opened = useRef(false);
  const statusRequest = useRef(null);
  const statusRevision = useRef(0);

  useEffect(() => {
    opened.current = false;
    setApp(initialApp || null);
    setError("");
    setStatusError("");
    setJobID("");
  }, [appId]);

  const ownJob = status.job?.kind?.startsWith(`ai-desktop-${appId}-`);
  const busy = submitting || Boolean(status.job?.running) || Boolean(jobID);

  const refresh = async (valid = () => true) => {
    const revision = statusRevision.current;
    const request = (statusRequest.current ??= api(`/ai-desktops/${appId}/status`));
    try {
      const value = await request;
      if (!valid() || revision !== statusRevision.current) return null;
      setApp(value);
      setStatusError("");
      return value;
    } catch (e) {
      if (valid() && revision === statusRevision.current) setStatusError(e.message);
      return null;
    } finally {
      if (statusRequest.current === request) statusRequest.current = null;
    }
  };

  const run = async (action) => {
    statusRevision.current++;
    statusRequest.current = null;
    setSubmitting(true);
    setError("");
    try {
      const job = await api(`/ai-desktops/${appId}/${action}`, {});
      setJobID(job.id);
    } catch (e) {
      setError(e.message);
    } finally {
      setSubmitting(false);
    }
  };

  useEffect(() => {
    if (!screenVisible || !status.vm_running || status.job?.running) return;
    let disposed = false;
    let timer;
    const poll = async () => {
      const value = await refresh(() => !disposed);
      if (disposed) return;
      if (value && !opened.current) {
        opened.current = true;
        if (value.installed && !value.running && !value.failed && !jobID) {
          await run("start");
        }
      }
      if (!disposed) timer = setTimeout(poll, 5000);
    };
    poll();
    return () => {
      disposed = true;
      clearTimeout(timer);
    };
  }, [appId, screenVisible, status.vm_running, status.job?.running]);

  useEffect(() => {
    if (!jobID || status.job?.id !== jobID || status.job.running) return;
    setJobID("");
    onCatalogRefresh?.();
    if (status.job.error) {
      setError(status.job.error);
      return;
    }
    if ([`ai-desktop-${appId}-install`, `ai-desktop-${appId}-update`].includes(status.job.kind)) {
      void run("start");
    } else {
      void refresh();
    }
  }, [status.job]);

  const title = app?.name || appId;
  const vncPrefix = `/ai-desktops/${appId}`;

  return (
    <section className="claude-desktop-view" aria-label={title}>
      <DesktopToolbar
        desktop={app}
        platform="Linux"
        title={title}
        zoom={zoom}
        setZoom={setZoom}
        onConfirm={onConfirm}
        onStop={() => run("stop")}
        busy={busy}
        refresh={() => refresh()}
        sidebarCollapsed={sidebarCollapsed}
        onExpand={onExpand}
      >
        {app?.installed && (
          <button
            className="quiet-button"
            disabled={busy || !app.arch_supported}
            onClick={async () => {
              if (
                !app.running ||
                (await onConfirm({
                  title: `更新 ${title}？`,
                  body: `请先保存应用内的工作。更新会关闭 ${title}，重新下载安装最新版本后再打开；登录资料继续保留。`,
                  confirm: "更新应用",
                }))
              ) {
                void run("update");
              }
            }}
          >
            {tr("更新应用")}
          </button>
        )}
      </DesktopToolbar>

      {(error || statusError || app?.failed) && (
        <div className="connection-banner" role="alert">
          <SafetyMessage
            message={
              error ||
              statusError ||
              (app?.failure_reason === "oom-kill"
                ? `环境内存不足，${title} 已被系统关闭。请在设置中检查计算资源后重新打开。`
                : `${title} 未能启动，可重试打开或修复安装。`)
            }
          />
          <button onClick={onDetails}>{tr("操作详情")}</button>
        </div>
      )}

      {busy && ownJob && (
        <div className="desktop-progress" role="status">
          <RefreshCw size={16} className="spin" />
          {status.job.message}
          <button className="text-button" onClick={onDetails}>
            {tr("查看进度")}
          </button>
        </div>
      )}

      <div className="desktop-screen">
        {app?.running && status.vm_running ? (
          <RemoteView
            key={appId}
            ref={remoteRef}
            desktop
            vncPrefix={vncPrefix}
            zoom={zoom}
            visible={visible}
            keyboard={status.keyboard}
            onStatus={onStatus}
            onFallback={onFallback}
          />
        ) : (
          <div className="desktop-empty">
            <Monitor size={36} />
            <h2>{app?.installed ? title : `${tr("在 Cage 中使用")} ${title}`}</h2>
            {app?.description && <p>{app.description}</p>}
            {app && !app.arch_supported && (
              <p className="desktop-note">
                {tr("当前客机架构暂不提供此应用的官方安装包。支持架构：")} {(app.supported_arches || []).join(", ")}
              </p>
            )}
            <div className="desktop-actions">
              {app && (
                <button
                  className="primary"
                  disabled={busy || !status.vm_running || (!app.installed && !app.arch_supported)}
                  onClick={() => run(app.installed ? "start" : "install")}
                >
                  {app.installed ? <Monitor size={16} /> : <Download size={16} />}
                  {app.installed ? `${tr("打开")} ${title}` : tr("下载并安装")}
                </button>
              )}
              {app?.installed && (
                <button
                  className="quiet-button"
                  disabled={busy || !app.arch_supported}
                  onClick={() => run("install")}
                >
                  {tr("更新 / 修复安装")}
                </button>
              )}
              {!app && (
                <button className="quiet-button" disabled={busy} onClick={() => refresh()}>
                  {tr("重新读取状态")}
                </button>
              )}
            </div>
            <p className="desktop-note">
              {tr("应用运行在独立的 Cage 图形会话中，所有流量自动走 Cage 代理，关闭应用后释放图形资源并保留登录数据。")}
            </p>
          </div>
        )}
      </div>
    </section>
  );
}
