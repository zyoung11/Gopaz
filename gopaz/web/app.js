const $ = (sel) => document.querySelector(sel);
const $$ = (sel) => Array.from(document.querySelectorAll(sel));

const I18N = {
  en: {
    "status.ready": "Ready",
    "status.loading": "Loading video…",
    "status.running": "Running",
    "status.done": "Done",
    "status.failed": "Failed",
    "status.connecting": "Disconnected",
    "input.title": "Input",
    "input.change": "Change",
    "input.select": "Select video",
    "meta.resolution": "Resolution",
    "meta.fps": "Frame rate",
    "meta.frames": "Frames",
    "meta.duration": "Duration",
    "meta.audio": "Audio",
    "meta.noAudio": "No audio",
    "filters.title": "Filters",
    "compute.title": "Compute",
    "compute.provider": "Provider",
    "compute.device": "Device",
    "filters.enhance": "Enhance",
    "filters.interp": "Interpolate",
    "stage.title": "Import a video to start",
    "stage.desc": "Pick a local video to begin.",
    "stage.select": "Select video",
    "preview.desc": "Open the output video after processing",
    "player.original": "Original",
    "player.processed": "Processed",
    "player.side": "Side by side",
    "player.divider": "Divider compare",
    "player.play": "Play",
    "player.pause": "Pause",
    "player.noPreview": "No preview yet. Click Preview to render the current range.",
    "player.browserError": "The browser cannot play this file. Try the processed preview.",
    "player.output": "Output",
    "settings.title": "Settings",
    "settings.output": "Output",
    "settings.encoder": "Encoder",
    "settings.container": "Container",
    "settings.quality": "Quality",
    "settings.rateControl": "Rate control",
    "settings.bitrate": "Bitrate Mbps",
    "settings.audio": "Audio",
    "settings.audioBitrate": "Audio bitrate kbps",
    "settings.gpu": "GPU",
    "settings.threads": "Threads",
    "settings.workers": "Workers",
    "settings.vram": "VRAM limit GB",
    "settings.qp": "QP override",
    "settings.instances": "Interp sessions",
    "settings.gpuVram": "Aion tile budget GB",
    "settings.overlap": "Overlap",
    "settings.kcolor": "keepColor",
    "settings.scene": "Scene threshold",
    "settings.start": "Start frame",
    "settings.end": "End frame",
    "settings.advanced": "Advanced",
    "settings.scale": "Scale",
    "settings.params": "Parameters",
    "settings.factor": "Slow motion",
    "settings.sensitivity": "Sensitivity",
    "settings.enhance": "Enhance",
    "settings.interp": "Interpolate",
    "settings.on": "On",
    "settings.off": "Off",
    "settings.auto": "Auto",
    "settings.cpu": "CPU",
    "settings.audioCopy": "Copy",
    "settings.audioNone": "None",
    "output.saveTo": "Save to",
    "output.autoName": "Auto name",
    "output.customName": "Custom name",
    "output.reset": "Auto",
    "output.placeholder": "Default: same folder as input",
    "output.namePlaceholder": "File name",
    "output.pickFolder": "Select folder",
    "output.savesAs": "Saves as",
    "cli.label": "Equivalent CLI command",
    "log.show": "Logs",
    "log.hide": "Hide logs",
    "task.start": "Start",
    "task.preview": "Preview",
    "task.previewing": "Previewing…",
    "toast.previewReady": "Preview ready",
    "toast.previewShort": "Not enough frames for a preview",
    "task.processing": "Processing…",
    "task.ready": "Ready",
    "task.preparing": "Preparing",
    "task.enhancing": "Enhancing",
    "task.interpolating": "Interpolating",
    "task.inProgress": "In progress",
    "task.elapsed": "Elapsed",
    "task.live": "Live",
    "compile.title": "Precompiling",
    "compile.tip": "Building the compute engine for this model and shape. The first time takes a few minutes. The result is cached in the cache folder and the same model and shape never compiles again. Changing model, resolution, scale or provider may trigger a new build. Deleting the cache folder frees space and only costs one rebuild.",
    "models.none": "No model files found in the models folder",
    "task.done": "Done",
    "task.cancelled": "Cancelled",
    "task.failed": "Failed",
    "task.eta": "ETA",
    "task.fpsUnit": "fps",
    "task.cancel": "Cancel",
    "confirm.stopTitle": "Stop the task?",
    "confirm.stopText": "Progress so far will be discarded.",
    "confirm.stop": "Stop",
    "task.reveal": "Open output location",
    "toast.configLoading": "Configuration is still loading",
    "toast.selectPath": "Enter a video path",
    "toast.taskRunning": "A job is running. Wait for it to finish or cancel it.",
    "toast.done": "Processing complete",
    "toast.dropUnsupported": "The browser cannot read dragged file paths. Use Select video.",
    "lang.toggle": "Switch language",
    "model.note.proteus": "General enhancement · 6 parameters",
    "model.note.gaia": "High quality upscale · 5-frame temporal",
    "model.note.gaiacg": "Computer generated · 5-frame temporal",
    "model.note.apollo": "Fast interpolation · 8 subframes",
    "model.note.aion": "High quality interpolation · 16 subframes",
    "tooltip.model.proteus": "<p>Upscale video with multiple parameters for fine-tuning. Parameters include Compression reversion, detail recovery, sharpening, noise reduction, dehaloing, and antialiasing / deblurring. Use \"Comparison View\" when trying out the model to hone in on the best results.</p>",
    "tooltip.model.gaia": "<p>Upscale high quality video. May produce more details.</p>",
    "tooltip.model.gaia-cg": "<p>Upscale video with aliasing or moire patterns. May enhance more details.</p>",
    "tooltip.model.apollo": "<p>Specializes in non-linear motion and slightly blurry inputs. Generates 8 interpolated frames per run; much faster processing than Chronos when you need more generated frames.</p>",
    "tooltip.model.aion": "<p>Improves upon Apollo at handling large motion. Fewer tiling artifacts on high resolution videos. Supports up to 16x slow motion.</p>",
    "tooltip.param.deblur": "<p>Adjusts anti-aliasing and deblurring strength.</p><p>Negative values are better if the input video has aliasing artifacts such as moire patterns or staircasing. Positive values are better if the input video has more lens blurring than aliasing artifacts.</p>",
    "tooltip.param.denoise": "<p>Removes ISO noise from the input video. Higher values remove more noise but may also remove fine details.</p><p>Adjust this based on the amount of noise in the input video. Higher values on videos with low ISO noise may introduce artifacts.</p>",
    "tooltip.param.detail": "<p>Recovers fine texture and detail lost due to in-camera noise suppression.</p><p>Adjust this based on the amount of noise suppression in the camera. Higher values may introduce artifacts if the input video has little in-camera noise suppression.</p>",
    "tooltip.param.dehalo": "<p>Increase this if the input video has halo or ringing artifacts around strong edges caused by over-sharpening.</p><p>The value is relative to the amount of haloing in the input video. Values that are too high may cause additional artifacts.</p>",
    "tooltip.param.sharpen": "<p>Additional sharpening of the video. Use this if the input video looks too soft.</p><p>Adjust this based on the softness of the input video. If the input is already sharp, higher values will introduce artifacts.</p>",
    "tooltip.param.fix-compression": "<p>Reduces compression artifacts from codec encoding, such as blockiness or mosquito noise. Higher values are best for low bitrate videos.</p><p>Adjust this based on the amount of compression artifacts in the input. Higher values on a clean video will introduce more artifacts.</p>",
    "tooltip.param.scale": "<p>Output scale factor. 1x repairs the video without resizing; 2x and 4x upscale it.</p>",
    "tooltip.param.factor": "<p>Slow the playback speed of your input video for a slow-mo effect.</p><p>The frame interpolation filter calculates the number of frames from the output FPS and the slow motion factor. For example, 2x FPS and 3x slow motion produce 6x the input frame count.</p>",
    "tooltip.param.sensitivity": "<p>Raise the slider to increase sensitivity for removing duplicate or similar frames.</p><p>A higher setting may remove frames that are not duplicates but look very similar.</p>",
  },
  zh: {
    "status.ready": "就绪",
    "status.loading": "读取视频…",
    "status.running": "运行中",
    "status.done": "完成",
    "status.failed": "失败",
    "status.connecting": "连接失败",
    "input.title": "输入",
    "input.change": "更换",
    "input.select": "选择视频",
    "meta.resolution": "分辨率",
    "meta.fps": "帧率",
    "meta.frames": "帧数",
    "meta.duration": "时长",
    "meta.audio": "含音轨",
    "meta.noAudio": "无音轨",
    "filters.title": "滤镜",
    "compute.title": "计算",
    "compute.provider": "后端",
    "compute.device": "设备",
    "filters.enhance": "增强",
    "filters.interp": "插帧",
    "stage.title": "导入视频开始",
    "stage.desc": "选择一个本地视频开始。",
    "stage.select": "选择视频",
    "preview.desc": "处理完成后可打开输出视频",
    "player.original": "原画",
    "player.processed": "处理后",
    "player.side": "左右对比",
    "player.divider": "分割对比",
    "player.play": "播放",
    "player.pause": "暂停",
    "player.noPreview": "还没有预览，点击预览按钮处理当前区间。",
    "player.browserError": "浏览器无法播放该文件，可以查看处理后的预览。",
    "player.output": "输出",
    "settings.title": "设置",
    "settings.output": "输出",
    "settings.encoder": "编码器",
    "settings.container": "容器",
    "settings.quality": "质量",
    "settings.rateControl": "码率控制",
    "settings.bitrate": "码率 Mbps",
    "settings.audio": "音频",
    "settings.audioBitrate": "音频码率 kbps",
    "settings.gpu": "GPU",
    "settings.threads": "线程",
    "settings.workers": "Worker",
    "settings.vram": "显存上限 GB",
    "settings.qp": "QP 覆盖",
    "settings.instances": "插值会话",
    "settings.gpuVram": "Aion tile 预算 GB",
    "settings.overlap": "重叠",
    "settings.kcolor": "keepColor",
    "settings.scene": "场景阈值",
    "settings.start": "起始帧",
    "settings.end": "结束帧",
    "settings.advanced": "高级",
    "settings.scale": "倍率",
    "settings.params": "参数",
    "settings.factor": "慢放倍数",
    "settings.sensitivity": "灵敏度",
    "settings.enhance": "增强",
    "settings.interp": "插帧",
    "settings.on": "开",
    "settings.off": "关",
    "settings.auto": "自动",
    "settings.cpu": "CPU",
    "settings.audioCopy": "复制",
    "settings.audioNone": "无",
    "output.saveTo": "保存到",
    "output.autoName": "自动命名",
    "output.customName": "自定义命名",
    "output.reset": "自动",
    "output.placeholder": "默认与输入视频同目录",
    "output.namePlaceholder": "文件名",
    "output.pickFolder": "选择文件夹",
    "output.savesAs": "实际保存为",
    "cli.label": "当前设置对应的命令行",
    "log.show": "日志",
    "log.hide": "收起日志",
    "task.start": "开始处理",
    "task.preview": "预览",
    "task.previewing": "预览中…",
    "toast.previewReady": "预览完成",
    "toast.previewShort": "剩余帧数不足以生成预览",
    "task.processing": "处理中…",
    "task.ready": "就绪",
    "task.preparing": "准备中",
    "task.enhancing": "增强中",
    "task.interpolating": "插帧中",
    "task.inProgress": "进行中",
    "task.elapsed": "用时",
    "task.live": "实时",
    "compile.title": "预编译中",
    "compile.tip": "正在为当前模型和尺寸构建计算引擎，首次需要几分钟。构建结果缓存在 cache 文件夹，相同模型和尺寸再次运行无需重新编译；更换模型、分辨率、倍率或后端可能触发重新编译。删除 cache 文件夹可以释放空间，代价只是下次重新构建一次。",
    "models.none": "models 文件夹中未找到模型文件",
    "task.done": "完成",
    "task.cancelled": "已取消",
    "task.failed": "失败",
    "task.eta": "剩余",
    "task.fpsUnit": "帧/秒",
    "task.cancel": "取消",
    "confirm.stopTitle": "停止当前任务？",
    "confirm.stopText": "已完成的进度将被丢弃。",
    "confirm.stop": "停止",
    "task.reveal": "打开输出位置",
    "toast.configLoading": "配置尚未加载完成",
    "toast.selectPath": "请输入视频路径",
    "toast.taskRunning": "任务进行中，完成后才能更换输入",
    "toast.done": "处理完成",
    "toast.dropUnsupported": "浏览器无法读取拖入文件的路径，请点击「选择视频」",
    "lang.toggle": "切换语言",
    "model.note.proteus": "通用增强 · 6 参数",
    "model.note.gaia": "高质量放大 · 5 帧时序",
    "model.note.gaiacg": "计算机图形 · 5 帧时序",
    "model.note.apollo": "快速插帧 · 8 子帧",
    "model.note.aion": "高质量插帧 · 16 子帧",
    "tooltip.model.proteus": "<p>通过多个参数精调放大视频。参数包括压缩修复、细节恢复、锐化、降噪、去光晕和抗锯齿/去模糊。试用模型时可使用对比视图找到最佳结果。</p>",
    "tooltip.model.gaia": "<p>放大高质量视频，可恢复更多细节。</p>",
    "tooltip.model.gaia-cg": "<p>放大存在锯齿或摩尔纹的视频，可增强更多细节。</p>",
    "tooltip.model.apollo": "<p>专门处理非线性运动和略微模糊的输入。每次运行生成 8 个插值帧；在需要生成更多帧时比 Chronos 快得多。</p>",
    "tooltip.model.aion": "<p>在大运动处理上优于 Apollo。在高分辨率视频上分块伪影更少。支持最高 16 倍慢放。</p>",
    "tooltip.param.deblur": "<p>调整抗锯齿和去模糊强度。</p><p>输入视频存在摩尔纹或阶梯等锯齿伪影时，负值更合适；镜头模糊多于锯齿时，正值更合适。</p>",
    "tooltip.param.denoise": "<p>移除输入视频中的 ISO 噪点。数值越高移除的噪点越多，但也可能移除细节。</p><p>应根据输入视频的噪点程度调整。低噪点视频使用较高数值可能引入伪影。</p>",
    "tooltip.param.detail": "<p>用于恢复因相机降噪而丢失的细腻纹理和细节。</p><p>应根据相机降噪程度调整。如果输入视频几乎没有相机降噪，较高数值可能引入伪影。</p>",
    "tooltip.param.dehalo": "<p>当输入视频在强边缘周围因过度锐化出现光晕或振铃伪影时提高该值。</p><p>数值应相对于输入视频的光晕程度。数值过高可能导致额外伪影。</p>",
    "tooltip.param.sharpen": "<p>对视频进行额外锐化。输入视频看起来偏软时使用。</p><p>应根据输入视频的软度调整。如果输入视频已经很锐利，较高数值会引入伪影。</p>",
    "tooltip.param.fix-compression": "<p>减少编码造成的压缩伪影，例如块效应或蚊式噪声。低码率视频适合较高数值。</p><p>应根据输入视频的压缩伪影程度调整。干净视频使用较高数值会引入更多伪影。</p>",
    "tooltip.param.scale": "<p>输出倍率。1x 只修复不改变尺寸；2x 和 4x 会放大视频。</p>",
    "tooltip.param.factor": "<p>降低输入视频的播放速度以获得慢动作效果。</p><p>插帧滤镜会根据输出 FPS 和慢放倍数计算要生成的帧数。例如输出 FPS 为 2 倍、慢放 3 倍时，总帧数为输入的 6 倍。</p>",
    "tooltip.param.sensitivity": "<p>提高滑块可增加对重复帧或相似帧的移除灵敏度。</p><p>数值过高可能移除并非重复但看起来非常相似的帧。</p>",
  },
};

let lang = "en";
if (lang !== "en" && lang !== "zh") lang = "en";

function t(key) {
  return (I18N[lang] && I18N[lang][key]) || I18N.en[key] || key;
}

function modelNote(key) {
  return t(`model.note.${key.replace(/-/g, "")}`);
}

function basename(path) {
  return (path || "").split(/[\\/]/).pop() || "";
}

const FACTORS = [2, 3, 4, 6, 8, 10, 12, 16];

const state = {
  config: null,
  input: null,
  mode: "enhance",
  model: "proteus",
  provider: "cuda",
  device: "",
  videoDevice: "sw",
  encoder: "",
  container: "",
  quality: "High",
  rateControl: "cq",
  audioCodec: "copy",
  outputDir: "",
  outputPath: "",
  outputName: "",
  outputError: "",
  autoOutputName: "",
  customName: false,
  settings: {
    scale: 2,
    factor: 2,
    sensitivity: 10,
    logHeight: 0,
    params: {},
  },
  busy: false,
  picking: false,
  job: null,
  pollTimer: null,
  logsShown: 0,
  previewSeq: 0,
  previewTimer: null,
  statusKey: "status.ready",
  playerMode: "original",
  preview: null,
  seeking: false,
  draggingDivider: false,
  divider: 50,
  tooltipTarget: null,
};

function currentModel() {
  return state.config.models.find((m) => m.key === state.model);
}

function encoderById(id) {
  return state.config.encoders.find((e) => e.id === id);
}

function setStatusKey(key) {
  state.statusKey = key;
  $("#serverState").textContent = t(key);
}

function updateOutputLabel() {
  $("#outputNameLabel").textContent = state.customName ? t("output.customName") : t("output.autoName");
  $("#resetOutputName").hidden = !state.customName;
}

function renderOutputHint() {
  const hint = $("#outputNameHint");
  if (state.outputError) {
    hint.textContent = state.outputError;
    hint.classList.add("error");
    return;
  }
  if (state.outputName) {
    hint.textContent = `${t("output.savesAs")} ${state.outputName}`;
    hint.classList.remove("error");
    return;
  }
  hint.textContent = "";
  hint.classList.remove("error");
}

function applyI18n() {
  hideTooltip();
  document.documentElement.lang = lang === "zh" ? "zh-CN" : "en";
  $$("[data-i18n]").forEach((el) => {
    el.textContent = t(el.dataset.i18n);
  });
  $$("[data-i18n-placeholder]").forEach((el) => {
    el.placeholder = t(el.dataset.i18nPlaceholder);
  });
  $$("[data-i18n-title]").forEach((el) => {
    el.title = t(el.dataset.i18nTitle);
  });
  $("#languageButton").textContent = lang === "zh" ? "EN" : "中文";
  $("#languageButton").title = t("lang.toggle");
  $("#logButton").textContent = $("#logWrap").hidden ? t("log.show") : t("log.hide");
  $("#startButton").textContent = state.busy ? t("task.processing") : t("task.start");
  updateOutputLabel();
  renderOutputHint();
  setStatusKey(state.statusKey);
  if (state.config) {
    renderFilters();
    selectModel(state.model, false);
  }
  if (state.input) renderInput();
  if (state.job) renderJob(state.job);
  else resetTaskLabels();
  renderCliHint();
  updatePreviewButton();
  updatePlayerModeUI();
}

async function fetchJSON(url, opts = {}) {
  const res = await fetch(url, opts);
  const text = await res.text();
  let data = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = null;
    }
  }
  if (!res.ok) {
    throw new Error((data && data.error) || `request failed ${res.status}`);
  }
  return data;
}

function postJSON(url, body) {
  return fetchJSON(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

let toastTimer = null;
function toast(msg, isError = false) {
  const el = $("#toast");
  el.textContent = msg;
  el.classList.toggle("error", isError);
  el.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => {
    el.hidden = true;
  }, 4500);
}

function showTooltip(el) {
  if (!el || !el.dataset.tooltipKey) return;
  if (state.tooltipTarget === el) return;
  const body = t(el.dataset.tooltipKey);
  if (!body || body === el.dataset.tooltipKey) return;
  const tip = $("#tooltip");
  tip.innerHTML = "";
  const title = document.createElement("span");
  title.className = "tooltip-title";
  title.textContent = el.dataset.tooltipTitle || el.textContent.trim();
  const content = document.createElement("div");
  content.className = "tooltip-body";
  content.innerHTML = body;
  tip.append(title, content);
  tip.hidden = false;
  const rect = el.getBoundingClientRect();
  const margin = 8;
  let left = rect.left;
  let top = rect.bottom + 6;
  const width = tip.offsetWidth;
  const height = tip.offsetHeight;
  if (left + width > window.innerWidth - margin) left = window.innerWidth - width - margin;
  if (left < margin) left = margin;
  if (top + height > window.innerHeight - margin) top = rect.top - height - 6;
  if (top < margin) top = margin;
  tip.style.left = `${left}px`;
  tip.style.top = `${top}px`;
  state.tooltipTarget = el;
}

function hideTooltip() {
  const tip = $("#tooltip");
  if (!tip) return;
  tip.hidden = true;
  state.tooltipTarget = null;
}

function formatBytes(n) {
  if (!n || n <= 0) return "";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v >= 10 || i === 0 ? 0 : 1)} ${units[i]}`;
}

function formatDuration(seconds) {
  if (!Number.isFinite(seconds) || seconds <= 0) return "—";
  const s = Math.round(seconds);
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const r = s % 60;
  if (h > 0) return `${h}:${String(m).padStart(2, "0")}:${String(r).padStart(2, "0")}`;
  return `${m}:${String(r).padStart(2, "0")}`;
}

function formatFPS(fps) {
  if (!Number.isFinite(fps) || fps <= 0) return "—";
  return Number.isInteger(fps) ? String(fps) : fps.toFixed(3).replace(/0+$/, "").replace(/\.$/, "");
}

function numberFrom(sel, fallback = 0) {
  const raw = $(sel).value.trim();
  if (raw === "") return fallback;
  const v = Number(raw);
  return Number.isFinite(v) ? v : fallback;
}

function translateProgress(label) {
  if (!label) return t("task.inProgress");
  if (label === "Enhancing" || label === "Enhance") return t("task.enhancing");
  if (label === "Interpolating" || label === "Interpolate") return t("task.interpolating");
  return label;
}

async function loadConfig() {
  const cfg = await fetchJSON("/api/config");
  state.config = {
    ...cfg,
    models: cfg.models.map((m) => ({ ...m, params: m.params || [] })),
  };
  const defaults = cfg.defaults || {};
  state.encoder = defaults.encoder || (cfg.encoders[0] && cfg.encoders[0].id) || "";
  state.quality = defaults.quality || "High";
  state.rateControl = defaults.rateControl || "cq";
  state.audioCodec = defaults.audioCodec || "copy";
  if (defaults.audioBitrate) $("#audioBitrateInput").value = defaults.audioBitrate;
  applySettings(cfg.app);

  renderFilters();
  populateEncoders();
  populateProviders();
  updateGpuOptions();
  $("#brandVersion").textContent = cfg.version || "";
  if (cfg.models.length > 0) {
    const saved = cfg.app && cfg.app.state && cfg.app.state.model;
    const key = cfg.models.some((m) => m.key === saved) ? saved : cfg.models.some((m) => m.key === "proteus") ? "proteus" : cfg.models[0].key;
    selectModel(key);
  } else {
    toast(t("models.none"), true);
  }
  applyI18n();
  resetTaskLabels();
  setStatusKey("status.ready");
  renderCliHint();
}

function renderFilters() {
  hideTooltip();
  if (!state.config) return;
  const host = $("#filterList");
  host.innerHTML = "";
  for (const kind of ["enhance", "interp"]) {
    const title = document.createElement("div");
    title.className = "group-title";
    title.textContent = kind === "enhance" ? t("filters.enhance") : t("filters.interp");
    host.append(title);
    for (const model of state.config.models.filter((m) => m.kind === kind)) {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "filter-item";
      btn.dataset.model = model.key;
      btn.dataset.tooltipKey = `tooltip.model.${model.key}`;
      btn.dataset.tooltipTitle = model.display;
      const icon = kind === "enhance" ? "#i-spark" : "#i-frames";
      btn.innerHTML = `
        <span class="filter-icon"><svg class="icon"><use href="${icon}"/></svg></span>
        <span class="filter-text"><b>${model.display}</b><small>${modelNote(model.key)}</small></span>`;
      btn.addEventListener("click", () => selectModel(model.key));
      host.append(btn);
    }
  }
  $$(".filter-item").forEach((el) => el.classList.toggle("active", el.dataset.model === state.model));
}

function selectModel(key, sync = true) {
  const model = state.config.models.find((m) => m.key === key);
  if (!model) return;
  state.model = key;
  state.mode = model.kind;

  if (model.kind === "enhance") {
    if (!model.scales.includes(state.settings.scale)) {
      state.settings.scale = model.scales.includes(2) ? 2 : model.scales[0];
    }
    for (const p of model.params) {
      if (!(p.key in state.settings.params)) state.settings.params[p.key] = 0;
    }
  }

  $$(".filter-item").forEach((el) => el.classList.toggle("active", el.dataset.model === key));
  $("#modelBadge").textContent = model.display;

  $("#kcolorField").hidden = model.kind !== "enhance";
  $("#sceneField").hidden = model.kind !== "enhance";
  $("#gpuVramField").hidden = model.kind !== "interp";
  $("#instancesField").hidden = model.kind !== "interp";

  renderModelSettings();
  updateDeviceOptions();
  renderCliHint();
  if (sync) syncOutputSoon();
  updateStartState();
}

function renderModelSettings() {
  hideTooltip();
  const model = currentModel();
  if (!model) return;
  const host = $("#modelSettings");
  host.innerHTML = "";

  if (model.kind === "enhance") {
    const group = document.createElement("div");
    group.className = "control-group";
    const label = document.createElement("div");
    label.className = "control-label";
    label.textContent = t("settings.scale");
    label.dataset.tooltipKey = "tooltip.param.scale";
    label.dataset.tooltipTitle = t("settings.scale");
    group.append(label);
    group.append(
      segmented(
        model.scales.map((v) => ({ value: v, label: `${v}x` })),
        state.settings.scale,
        (v) => {
          state.settings.scale = v;
          renderCliHint();
        }
      )
    );
    host.append(group);

    if (model.params.length > 0) {
      const params = document.createElement("div");
      params.className = "control-group";
      const plabel = document.createElement("div");
      plabel.className = "control-label";
      plabel.textContent = t("settings.params");
      params.append(plabel);

      for (const p of model.params) {
        params.append(
          sliderRow(p.guiName, -100, 100, 1, state.settings.params[p.key] ?? 0, (v) => {
            state.settings.params[p.key] = v;
          }, `tooltip.param.${p.key}`)
        );
      }
      host.append(params);
    }
    return;
  }

  const factorGroup = document.createElement("div");
  factorGroup.className = "control-group";
  const factorLabel = document.createElement("div");
  factorLabel.className = "control-label";
  factorLabel.textContent = t("settings.factor");
  factorLabel.dataset.tooltipKey = "tooltip.param.factor";
  factorLabel.dataset.tooltipTitle = t("settings.factor");
  factorGroup.append(factorLabel);
  factorGroup.append(
    segmented(
      FACTORS.map((v) => ({ value: v, label: `${v}x` })),
      state.settings.factor,
      (v) => {
        state.settings.factor = v;
        renderCliHint();
      }
    )
  );
  host.append(factorGroup);

  const sensGroup = document.createElement("div");
  sensGroup.className = "control-group";
  const sensLabel = document.createElement("div");
  sensLabel.className = "control-label";
  sensLabel.textContent = t("settings.sensitivity");
  sensLabel.dataset.tooltipKey = "tooltip.param.sensitivity";
  sensLabel.dataset.tooltipTitle = t("settings.sensitivity");
  sensGroup.append(sensLabel);
  sensGroup.append(
    sliderRow(t("settings.sensitivity"), 0, 100, 1, state.settings.sensitivity, (v) => {
      state.settings.sensitivity = v;
      renderCliHint();
    }, "tooltip.param.sensitivity")
  );
  host.append(sensGroup);
}

function segmented(options, current, onChange) {
  const box = document.createElement("div");
  box.className = "segmented";
  for (const opt of options) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.textContent = opt.label;
    if (opt.value === current) btn.classList.add("active");
    btn.addEventListener("click", () => {
      Array.from(box.children).forEach((el) => el.classList.remove("active"));
      btn.classList.add("active");
      onChange(opt.value);
    });
    box.append(btn);
  }
  return box;
}

function sliderRow(label, min, max, step, value, onChange, tooltipKey) {
  const row = document.createElement("div");
  row.className = "slider-row";

  const name = document.createElement("label");
  name.textContent = label;
  if (tooltipKey) {
    name.dataset.tooltipKey = tooltipKey;
    name.dataset.tooltipTitle = label;
  }

  const range = document.createElement("input");
  range.type = "range";
  range.min = String(min);
  range.max = String(max);
  range.step = String(step);
  range.value = String(value);

  const num = document.createElement("input");
  num.type = "number";
  num.min = String(min);
  num.max = String(max);
  num.step = String(step);
  num.value = String(value);

  const apply = (v) => {
    const n = Math.max(min, Math.min(max, Number(v)));
    const clean = Number.isFinite(n) ? n : 0;
    range.value = String(clean);
    num.value = String(clean);
    onChange(clean);
  };
  range.addEventListener("input", () => apply(range.value));
  num.addEventListener("change", () => apply(num.value));

  row.append(name, range, num);
  return row;
}

function populateEncoders() {
  const select = $("#encoderSelect");
  select.innerHTML = "";
  for (const enc of state.config.encoders) {
    const opt = document.createElement("option");
    opt.value = enc.id;
    opt.textContent = enc.desc;
    select.append(opt);
  }
  if (!state.config.encoders.some((e) => e.id === state.encoder)) {
    state.encoder = state.config.encoders[0].id;
  }
  select.value = state.encoder;
  updateContainers();
  updateQualityState();
  updateRateControlState();
  updateAudioState();
}

function providerById(id) {
  return (state.config.providers || []).find((p) => p.id === id);
}

function populateProviders() {
  const select = $("#providerSelect");
  select.innerHTML = "";
  for (const provider of state.config.providers || []) {
    const opt = document.createElement("option");
    opt.value = provider.id;
    opt.textContent = provider.display;
    select.append(opt);
  }
  if (!providerById(state.provider)) {
    state.provider = (state.config.providers && state.config.providers[0] && state.config.providers[0].id) || "cuda";
  }
  select.value = state.provider;
  updateDeviceOptions();
}

function updateDeviceOptions() {
  const provider = providerById(state.provider);
  const field = $("#deviceField");
  const select = $("#deviceSelect");
  const model = currentModel();
  const noNPU = !!(model && model.npuUnsupported);
  const devices = ((provider && provider.devices) || []).filter(
    (d) => !(noNPU && d.id.toUpperCase() === "NPU"),
  );
  field.hidden = devices.length === 0;
  select.innerHTML = "";
  if (devices.length === 0) {
    state.device = "";
    return;
  }
  for (const device of devices) {
    const opt = document.createElement("option");
    opt.value = device.id;
    opt.textContent = device.display;
    select.append(opt);
  }
  if (!devices.some((d) => d.id === state.device)) {
    state.device = devices[0].id;
  }
  select.value = state.device;
}

function updateGpuOptions() {
  const select = $("#gpuSelect");
  const current = select.value;
  select.innerHTML = "";
  const autoOpt = document.createElement("option");
  autoOpt.value = "-2";
  autoOpt.dataset.i18n = "settings.auto";
  autoOpt.textContent = t("settings.auto");
  select.append(autoOpt);
  const cpuOpt = document.createElement("option");
  cpuOpt.value = "-1";
  cpuOpt.dataset.i18n = "settings.cpu";
  cpuOpt.textContent = t("settings.cpu");
  select.append(cpuOpt);
  const gpus = (state.config && state.config.gpus) || [];
  if (gpus.length === 0) {
    for (let i = 0; i < 4; i++) {
      const opt = document.createElement("option");
      opt.value = String(i);
      opt.textContent = `Device ${i}`;
      select.append(opt);
    }
  } else {
    for (const gpu of gpus) {
      const opt = document.createElement("option");
      opt.value = gpu.id;
      opt.textContent = gpu.display;
      select.append(opt);
    }
  }
  if ([...select.options].some((o) => o.value === current)) select.value = current;
}

function selectProvider(id) {
  state.provider = id;
  state.device = "";
  updateDeviceOptions();
  renderCliHint();
}

function updateContainers() {
  const enc = encoderById(state.encoder);
  const select = $("#containerSelect");
  select.innerHTML = "";
  for (const ext of enc.extensions) {
    const opt = document.createElement("option");
    opt.value = ext;
    opt.textContent = ext.toUpperCase();
    select.append(opt);
  }
  if (!enc.extensions.includes(state.container)) {
    state.container = enc.extensions.includes("mp4") ? "mp4" : enc.extensions[0];
  }
  select.value = state.container;
}

function updateQualityState() {
  const enc = encoderById(state.encoder);
  $("#qualitySelect").disabled = !enc.hasCqp;
  $("#qualitySelect").value = state.quality;
}

function updateRateControlState() {
  const enc = encoderById(state.encoder);
  const select = $("#rateControlSelect");
  select.disabled = !enc.rateControl;
  if (!enc.rateControl) state.rateControl = "cq";
  select.value = state.rateControl;
  $("#bitrateField").hidden = state.rateControl === "cq";
}

function updateAudioState() {
  $("#audioSelect").value = state.audioCodec;
  $("#audioBitrateField").hidden = state.audioCodec !== "aac" && state.audioCodec !== "ac3";
}

function updateStartState() {
  const btn = $("#startButton");
  btn.disabled = !state.input || state.busy || !state.outputPath;
  btn.hidden = state.busy;
  btn.textContent = state.busy ? t("task.processing") : t("task.start");
  updatePreviewButton();
}

function renderCliHint() {
  const mode = state.mode === "enhance" ? "upscale" : "interpolate";
  const input = state.input ? state.input.name : "in.mp4";
  const output = state.outputPath ? basename(state.outputPath) : `out.${state.container || "mp4"}`;
  const parts = [
    `gopaz ${mode}`,
    `-i "${input}"`,
    `-o "${output}"`,
    `--model ${state.model}`,
  ];
  if (state.mode === "enhance") {
    parts.push(`--scale ${state.settings.scale}`, `--kcolor ${$("#kcolorSelect").value}`);
  } else {
    parts.push(`--factor ${state.settings.factor}`, `--sensitivity ${state.settings.sensitivity}`);
  }
  parts.push(`--encoder ${state.encoder}`);
  if (state.provider !== "cuda") parts.push(`--provider ${state.provider}`);
  if (state.device) parts.push(`--device ${state.device}`);
  if (state.videoDevice && state.videoDevice !== "sw") parts.push(`--video-device ${state.videoDevice}`);
  const line = parts.join(" ");
  const el = $("#cliHint");
  el.textContent = line;
  el.title = line;
}

function syncOutputSoon() {
  clearTimeout(state.previewTimer);
  state.previewTimer = setTimeout(syncOutput, 160);
}

async function syncOutput() {
  if (!state.config || state.busy) return;
  if (!state.input) {
    state.outputPath = "";
    state.outputName = "";
    state.outputError = "";
    state.autoOutputName = "";
    $("#outputNameInput").value = "";
    renderOutputHint();
    updateStartState();
    renderCliHint();
    return;
  }
  const seq = ++state.previewSeq;
  try {
    const res = await postJSON("/api/preview-output", buildRequest());
    if (seq !== state.previewSeq) return;
    state.outputPath = res.path;
    state.outputName = res.name;
    state.outputError = "";
    if (!state.customName) {
      state.autoOutputName = res.name;
      if (document.activeElement !== $("#outputNameInput")) {
        $("#outputNameInput").value = res.name;
      }
    }
    renderOutputHint();
  } catch (err) {
    if (seq !== state.previewSeq) return;
    state.outputPath = "";
    state.outputName = "";
    state.outputError = err.message;
    renderOutputHint();
  }
  updateOutputLabel();
  updateStartState();
  renderCliHint();
}

function buildRequest() {
  const params = {};
  for (const p of currentModel().params || []) {
    params[p.key] = state.settings.params[p.key] ?? 0;
  }
  return {
    input: state.input ? state.input.path : "",
    outputDir: $("#outputDirInput").value.trim(),
    outputName: state.customName ? $("#outputNameInput").value.trim() : "",
    mode: state.mode,
    model: state.model,
    scale: state.settings.scale,
    factor: state.settings.factor,
    params,
    encoder: state.encoder,
    container: state.container,
    quality: state.quality,
    rateControl: state.rateControl,
    bitrate: numberFrom("#bitrateInput"),
    audioCodec: state.audioCodec,
    audioBitrate: numberFrom("#audioBitrateInput"),
    gpu: numberFrom("#gpuSelect", -2),
    provider: state.provider,
    device: state.device,
    videoDevice: state.videoDevice,
    threads: numberFrom("#threadsInput"),
    workers: numberFrom("#workersInput"),
    vram: numberFrom("#vramInput"),
    gpuVram: numberFrom("#gpuVramInput"),
    overlap: numberFrom("#overlapInput"),
    kcolor: $("#kcolorSelect").value === "1",
    sceneThreshold: numberFrom("#sceneInput", 6.5),
    startFrame: numberFrom("#startInput", -1),
    endFrame: numberFrom("#endInput", -1),
    sensitivity: state.settings.sensitivity,
    rdt: -1,
  };
}

function buildSettings() {
  const req = buildRequest();
  return {
    lang,
    input: state.input ? state.input.path : "",
    outputDir: req.outputDir,
    outputName: $("#outputNameInput").value.trim(),
    customName: state.customName,
    mode: req.mode,
    model: req.model,
    scale: req.scale,
    factor: req.factor,
    params: req.params,
    provider: req.provider,
    device: req.device,
    videoDevice: state.videoDevice,
    gpu: req.gpu,
    encoder: req.encoder,
    container: req.container,
    quality: req.quality,
    rateControl: req.rateControl,
    bitrate: req.bitrate,
    qp: numberFrom("#qpInput", 0),
    audioCodec: req.audioCodec,
    audioBitrate: req.audioBitrate,
    threads: req.threads,
    workers: req.workers,
    vram: req.vram,
    gpuVram: req.gpuVram,
    overlap: req.overlap,
    kcolor: req.kcolor ? 1 : 0,
    sceneThreshold: req.sceneThreshold,
    startFrame: req.startFrame,
    endFrame: req.endFrame,
    instances: numberFrom("#instancesInput", 0),
    sensitivity: req.sensitivity,
    logHeight: state.settings.logHeight || Math.round($("#logPanel").getBoundingClientRect().height),
    rdt: req.rdt,
  };
}

let settingsTimer = 0;

function syncSettingsSoon() {
  clearTimeout(settingsTimer);
  settingsTimer = setTimeout(saveSettings, 700);
}

async function saveSettings() {
  if (!state.config) return;
  try {
    await postJSON("/api/settings", buildSettings());
  } catch {}
}

function applySettings(app) {
  const s = (app && app.state) || {};
  if (s.lang === "zh" || s.lang === "en") lang = s.lang;
  state.outputDir = s.outputDir || "";
  state.outputName = s.outputName || "";
  state.customName = !!s.customName;
  $("#outputDirInput").value = state.outputDir;
  $("#outputNameInput").value = state.outputName;
  if (s.model) state.model = s.model;
  if (s.provider) state.provider = s.provider;
  state.device = s.device || "";
  if (s.videoDevice) {
    state.videoDevice = s.videoDevice;
  }
  if (s.encoder) state.encoder = s.encoder;
  if (s.container) state.container = s.container;
  if (s.quality) state.quality = s.quality;
  if (s.rateControl) state.rateControl = s.rateControl;
  if (s.audioCodec) state.audioCodec = s.audioCodec;
  if (s.scale) state.settings.scale = s.scale;
  if (s.factor) state.settings.factor = s.factor;
  if (s.sensitivity !== undefined) state.settings.sensitivity = s.sensitivity;
  if (s.params) state.settings.params = s.params;
  $("#bitrateInput").value = s.bitrate ?? 20;
  $("#audioBitrateInput").value = s.audioBitrate ?? 320;
  $("#gpuSelect").value = String(s.gpu ?? -2);
  $("#threadsInput").value = s.threads ?? 0;
  $("#workersInput").value = s.workers ?? 0;
  $("#vramInput").value = s.vram ?? 0;
  $("#qpInput").value = s.qp ?? 0;
  $("#instancesInput").value = s.instances ?? 0;
  $("#gpuVramInput").value = s.gpuVram ?? 0;
  $("#overlapInput").value = s.overlap ?? 0;
  $("#kcolorSelect").value = s.kcolor ? "1" : "0";
  $("#sceneInput").value = s.sceneThreshold ?? 6.5;
  $("#startInput").value = s.startFrame ?? -1;
  $("#endInput").value = s.endFrame ?? -1;
  if (s.logHeight) {
    state.settings.logHeight = s.logHeight;
    $("#logPanel").style.height = `${s.logHeight}px`;
  }
}

async function pickInput() {
  if (state.busy || state.picking) return;
  if (!state.config) {
    toast(t("toast.configLoading"), true);
    return;
  }
  state.picking = true;
  try {
    const res = await postJSON("/api/pick/file", {});
    if (res && res.path) await loadInput(res.path);
  } catch (err) {
    toast(err.message, true);
  } finally {
    state.picking = false;
  }
}

async function pickOutputDir() {
  if (state.busy) return;
  try {
    const res = await postJSON("/api/pick/folder", {});
    if (res && res.path) {
      state.outputDir = res.path;
      $("#outputDirInput").value = res.path;
      syncOutputSoon();
      syncSettingsSoon();
    }
  } catch (err) {
    toast(err.message, true);
  }
}

async function loadInput(path) {
  if (!state.config) {
    toast(t("toast.configLoading"), true);
    return;
  }
  if (state.busy) {
    toast(t("toast.taskRunning"), true);
    return;
  }
  const clean = path.trim().replace(/^"(.*)"$/, "$1");
  if (!clean) {
    toast(t("toast.selectPath"), true);
    return;
  }
  setStatusKey("status.loading");
  try {
    const info = await postJSON("/api/probe", { path: clean });
    state.input = info;
    state.preview = null;
    state.playerMode = "original";
    const originalVideo = $("#videoOriginal");
    originalVideo.pause();
    originalVideo.playbackRate = 1;
    originalVideo.src = mediaURL(info.path);
    originalVideo.load();
    const processedVideo = $("#videoProcessed");
    processedVideo.pause();
    processedVideo.removeAttribute("src");
    processedVideo.load();
    $("#playerMessage").hidden = true;
    setDivider(50);
    if (info.hasAudio) {
      if (state.audioCodec === "none") state.audioCodec = "copy";
    } else {
      state.audioCodec = "none";
    }
    updateAudioState();
    state.customName = false;
    state.autoOutputName = "";
    state.outputName = "";
    state.outputError = "";
    $("#outputNameInput").value = "";
    if (!state.outputDir) {
      state.outputDir = info.dir;
    }
    $("#outputDirInput").value = state.outputDir;
    syncSettingsSoon();
    renderInput();
    resetJobView();
    state.outputPath = "";
    updateStartState();
    syncOutputSoon();
    setStatusKey("status.ready");
  } catch (err) {
    setStatusKey("status.ready");
    toast(err.message, true);
  }
}

function renderInput() {
  const info = state.input;
  $("#inputEmpty").hidden = !!info;
  $("#inputCard").hidden = !info;
  $("#changeInputButton").hidden = !info;
  $("#stageEmpty").hidden = !!info;
  $("#player").hidden = !info;
  if (!info) return;

  const duration = info.duration > 0 ? info.duration : info.frames > 0 && info.fps > 0 ? info.frames / info.fps : 0;
  $("#inputName").textContent = info.name;
  $("#inputPathLabel").textContent = info.path;
  $("#inputPathLabel").title = info.path;
  $("#metaSize").textContent = `${info.width} × ${info.height}`;
  $("#metaFps").textContent = `${formatFPS(info.fps)} fps`;
  $("#metaFrames").textContent = info.frames > 0 ? String(info.frames) : "—";
  $("#metaDuration").textContent = formatDuration(duration);
  $("#metaAudio").textContent = info.hasAudio ? t("meta.audio") : t("meta.noAudio");
  $("#metaBytes").textContent = formatBytes(info.size);

  $("#previewFile").textContent = info.name;
  $("#previewFile").title = info.path;
  $("#previewPath").textContent = info.path;
  $("#previewPath").title = info.path;

  updatePlayerModeUI();
  updateTimeline();

}


function resetTaskLabels() {
  const track = $("#progressTrack");
  const fill = $("#progressFill");
  track.classList.remove("indeterminate");
  track.classList.remove("done");
  fill.style.transition = "none";
  fill.style.width = "0%";
  void fill.offsetWidth;
  fill.style.transition = "";
  $("#progressLabel").textContent = t("task.ready");
  $("#progressValue").textContent = "—";
}

async function startJob() {
  if (!state.input || state.busy) return;
  submitJob(buildRequest(), false);
}

async function submitJob(req, isPreview) {
  state.busy = true;
  updateStartState();
  resetJobView();
  $("#cancelButton").hidden = false;
  $("#cancelButton").disabled = false;
  $("#revealButton").hidden = true;
  $("#progressFill").style.width = "0%";
  $("#progressTrack").classList.remove("indeterminate");
  $("#progressLabel").textContent = t("task.preparing");
  $("#progressValue").textContent = "…";
  setStatusKey("status.running");
  try {
    const job = await postJSON("/api/jobs", req);
    state.job = job;
    if (!isPreview) {
      state.outputPath = job.output || state.outputPath;
      if (job.output) {
        const name = basename(job.output);
        if (!state.customName) state.autoOutputName = name;
        state.outputName = name;
        state.outputError = "";
        $("#outputNameInput").value = name;
        renderOutputHint();
      }
      updateOutputLabel();
      renderCliHint();
    }
    state.logsShown = 0;
    renderJob(job);
    startPolling(job.id);
  } catch (err) {
    state.busy = false;
    updateStartState();
    $("#cancelButton").hidden = true;
    $("#progressTrack").classList.remove("indeterminate");
    $("#progressLabel").textContent = t("task.failed");
    $("#progressValue").textContent = "0%";
    setStatusKey("status.ready");
    toast(err.message, true);
  }
}

function startPolling(id) {
  clearTimeout(state.pollTimer);
  const poll = async () => {
    if (!state.job || state.job.id !== id) return;
    let job = null;
    try {
      job = await fetchJSON(`/api/jobs/${id}`);
    } catch {
      state.pollTimer = setTimeout(poll, 1000);
      return;
    }
    state.job = job;
    renderJob(job);
    if (job.state === "running") {
      state.pollTimer = setTimeout(poll, 500);
      return;
    }
    finishJobUI(job);
  };
  state.pollTimer = setTimeout(poll, 300);
}

let compileTimer = 0;

function updateCompileOverlay(compiling) {
  const overlay = $("#compileOverlay");
  if (!compiling) {
    clearTimeout(compileTimer);
    compileTimer = 0;
    overlay.hidden = true;
    return;
  }
  if (compileTimer || !overlay.hidden) return;
  compileTimer = setTimeout(() => {
    compileTimer = 0;
    const job = state.job;
    const p = job && job.progress;
    if (job && job.state === "running" && p && p.phase === "compile") {
      overlay.hidden = false;
    }
  }, 3500);
}

function renderJob(job) {
  const p = job.progress || {};
  const total = p.total || job.total || 0;
  const running = job.state === "running";
  const track = $("#progressTrack");
  const prefix = job.preview ? `${t("task.preview")} · ` : "";
  const label = translateProgress(p.label);
  const compiling = running && p.phase === "compile";
  updateCompileOverlay(compiling);
  const compileShown = compiling && !$("#compileOverlay").hidden;
  const speed = p.speed || 0;
  const eta = p.eta || 0;
  const info = [speed > 0 ? `${speed.toFixed(1)} ${t("task.fpsUnit")}` : `— ${t("task.fpsUnit")}`];
  if (job.elapsed > 0) info.push(`${t("task.elapsed")} ${formatDuration(job.elapsed)}`);
  if (running) info.push(`${t("task.eta")} ${eta > 0 ? formatDuration(eta) : "—"}`);
  $("#progressValue").textContent = info.join(" · ");
  if (running && total <= 0) {
    track.classList.add("indeterminate");
    $("#progressFill").style.width = "";
    $("#progressLabel").textContent = compileShown ? t("compile.title") : `${prefix}${label}`;
  } else {
    track.classList.remove("indeterminate");
    const pct = total > 0 ? Math.min(100, (p.cur / total) * 100) : p.pct >= 0 ? p.pct : 0;
    $("#progressFill").style.width = `${pct}%`;
    const count = total > 0 ? ` ${p.cur}/${total}` : "";
    $("#progressLabel").textContent = compileShown ? t("compile.title") : `${prefix}${label}${count}`;
  }
  track.classList.toggle("done", job.state === "done");
  if (job.state === "done") {
    $("#progressFill").style.width = "100%";
    $("#progressLabel").textContent = `${prefix}${t("task.done")}`;
  } else if (job.state === "cancelled") {
    $("#progressLabel").textContent = `${prefix}${t("task.cancelled")}`;
  } else if (job.state === "error") {
    $("#progressLabel").textContent = `${prefix}${t("task.failed")}`;
  }

  $("#revealButton").hidden = job.preview || job.state !== "done";
  $("#cancelButton").hidden = job.state !== "running";
  appendLogs(job.logs || []);
  updatePreviewButton();
}

function logLevelLabel(level) {
  switch (level) {
    case "success":
      return "OK";
    case "warn":
      return "WARN";
    case "error":
      return "ERR";
    case "debug":
      return "DBG";
    default:
      return "INFO";
  }
}

function appendLogs(logs) {
  const panel = $("#logPanel");
  if (logs.length < state.logsShown) {
    panel.innerHTML = "";
    state.logsShown = 0;
  }
  const stick = panel.hidden || panel.scrollTop + panel.clientHeight >= panel.scrollHeight - 20;
  for (let i = state.logsShown; i < logs.length; i++) {
    const log = logs[i];
    const line = document.createElement("div");
    line.className = `log-line ${log.level || "info"}`;
    const time = document.createElement("span");
    time.className = "log-time";
    time.textContent = log.time || "";
    const level = document.createElement("span");
    level.className = "log-level";
    level.textContent = logLevelLabel(log.level);
    const msg = document.createElement("span");
    msg.className = "log-msg";
    msg.textContent = log.msg || "";
    line.append(time, level, msg);
    panel.append(line);
  }
  state.logsShown = logs.length;
  if (!panel.hidden && stick) panel.scrollTop = panel.scrollHeight;
}

function resetJobView() {
  state.job = null;
  state.logsShown = 0;
  clearTimeout(state.pollTimer);
  clearTimeout(state.previewTimer);
  $("#logPanel").innerHTML = "";
  $("#logButton").textContent = $("#logWrap").hidden ? t("log.show") : t("log.hide");
  $("#cancelButton").hidden = true;
  $("#revealButton").hidden = true;
  resetTaskLabels();
}

function finishJobUI(job) {
  clearTimeout(state.previewTimer);
  state.busy = false;
  updateStartState();
  if (job.preview) {
    $("#cancelButton").hidden = true;
    $("#revealButton").hidden = true;
    $("#progressTrack").classList.remove("indeterminate");
    if (job.state === "done") {
      loadPreview(job);
      $("#progressFill").style.width = "100%";
      $("#progressLabel").textContent = `${t("task.preview")} · ${t("task.done")}`;
      setStatusKey("status.done");
      toast(t("toast.previewReady"));
      return;
    }
    if (job.state === "cancelled") {
      $("#progressLabel").textContent = `${t("task.preview")} · ${t("task.cancelled")}`;
      setStatusKey("status.ready");
      return;
    }
    $("#progressLabel").textContent = `${t("task.preview")} · ${t("task.failed")}`;
    setStatusKey("status.failed");
    toast(job.error || t("task.failed"), true);
    return;
  }
  $("#cancelButton").hidden = true;
  $("#revealButton").hidden = job.state !== "done";
  $("#progressTrack").classList.remove("indeterminate");
  if (job.state === "done") {
    $("#progressFill").style.width = "100%";
    $("#progressLabel").textContent = t("task.done");
    if (job.output) {
      const name = basename(job.output);
      state.outputPath = job.output;
      if (!state.customName) state.autoOutputName = name;
      state.outputName = name;
      state.outputError = "";
      $("#outputNameInput").value = name;
      renderOutputHint();
      renderCliHint();
      loadPreview({
        output: job.output,
        previewStart: 0,
        previewSourceDuration: inputDuration(),
        full: true,
      });
    }
    setStatusKey("status.done");
    toast(t("toast.done"));
    return;
  }
  if (job.state === "cancelled") {
    $("#progressLabel").textContent = t("task.cancelled");
    setStatusKey("status.ready");
    return;
  }
  $("#progressLabel").textContent = t("task.failed");
  setStatusKey("status.failed");
  toast(job.error || t("task.failed"), true);
}

async function cancelJob() {
  if (!state.job || state.job.state !== "running") return;
  const ok = await confirmDialog(t("confirm.stopTitle"), t("confirm.stopText"), t("confirm.stop"));
  if (!ok) return;
  $("#cancelButton").disabled = true;
  try {
    await postJSON(`/api/jobs/${state.job.id}/cancel`, {});
  } catch (err) {
    $("#cancelButton").disabled = false;
    toast(err.message, true);
  }
}

function confirmDialog(title, text, okLabel) {
  return new Promise((resolve) => {
    const modal = $("#confirmModal");
    $("#confirmTitle").textContent = title;
    $("#confirmText").textContent = text || "";
    $("#confirmText").hidden = !text;
    $("#confirmOk").textContent = okLabel || "OK";
    modal.hidden = false;
    $("#confirmCancel").focus();
    const done = (ok) => {
      modal.hidden = true;
      document.removeEventListener("keydown", onKey, true);
      resolve(ok);
    };
    const onKey = (e) => {
      if (e.key === "Escape") {
        e.preventDefault();
        done(false);
      }
    };
    $("#confirmOk").onclick = () => done(true);
    $("#confirmCancel").onclick = () => done(false);
    modal.onclick = (e) => {
      if (e.target === modal) done(false);
    };
    document.addEventListener("keydown", onKey, true);
  });
}

async function revealOutput() {
  const path = (state.job && state.job.output) || state.outputPath;
  if (!path) return;
  try {
    await postJSON("/api/reveal", { path });
  } catch (err) {
    toast(err.message, true);
  }
}

function syncProgressDots() {
  const track = $("#progressTrack");
  const w = track.clientWidth;
  if (w > 0) {
    const cols = Math.max(1, Math.round(w / 5));
    track.style.setProperty("--dot-pitch", `${w / cols}px`);
  }
}

function toggleLogs() {
  const wrap = $("#logWrap");
  wrap.hidden = !wrap.hidden;
  $("#logButton").textContent = wrap.hidden ? t("log.show") : t("log.hide");
  if (!wrap.hidden) {
    const panel = $("#logPanel");
    panel.scrollTop = panel.scrollHeight;
  }
}

function bindEvents() {
  $("#languageButton").addEventListener("click", () => {
    lang = lang === "zh" ? "en" : "zh";
    applyI18n();
    syncSettingsSoon();
  });

  $("#pickInputButton").addEventListener("click", pickInput);
  $("#stagePickButton").addEventListener("click", pickInput);
  $("#changeInputButton").addEventListener("click", pickInput);

  $("#providerSelect").addEventListener("change", () => selectProvider($("#providerSelect").value));
  $("#deviceSelect").addEventListener("change", () => {
    state.device = $("#deviceSelect").value;
    renderCliHint();
  });

  $("#pickOutputDirButton").addEventListener("click", pickOutputDir);
  $("#outputDirInput").addEventListener("change", () => {
    state.outputDir = $("#outputDirInput").value.trim();
    syncOutputSoon();
    syncSettingsSoon();
  });
  $("#outputDirInput").addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      state.outputDir = $("#outputDirInput").value.trim();
      syncOutputSoon();
      syncSettingsSoon();
    }
  });

  document.addEventListener("change", syncSettingsSoon, true);
  document.addEventListener("input", syncSettingsSoon, true);
  document.addEventListener("click", syncSettingsSoon, true);
  window.addEventListener("beforeunload", () => {
    if (!state.config || !navigator.sendBeacon) return;
    navigator.sendBeacon("/api/settings", new Blob([JSON.stringify(buildSettings())], { type: "application/json" }));
  });

  const nameInput = $("#outputNameInput");
  nameInput.addEventListener("input", () => {
    state.customName = nameInput.value.trim() !== "";
    updateOutputLabel();
    renderCliHint();
    syncOutputSoon();
  });
  nameInput.addEventListener("blur", () => {
    if (!state.customName && state.autoOutputName) {
      nameInput.value = state.autoOutputName;
    } else if (state.customName && state.outputPath) {
      const name = basename(state.outputPath);
      if (name && nameInput.value !== name) nameInput.value = name;
    }
  });
  $("#resetOutputName").addEventListener("click", () => {
    state.customName = false;
    nameInput.value = state.autoOutputName;
    updateOutputLabel();
    renderCliHint();
    syncOutputSoon();
  });

  $("#encoderSelect").addEventListener("change", () => {
    state.encoder = $("#encoderSelect").value;
    updateContainers();
    updateQualityState();
    updateRateControlState();
    renderCliHint();
    syncOutputSoon();
  });
  $("#containerSelect").addEventListener("change", () => {
    state.container = $("#containerSelect").value;
    renderCliHint();
    syncOutputSoon();
  });
  $("#qualitySelect").addEventListener("change", () => {
    state.quality = $("#qualitySelect").value;
  });
  $("#rateControlSelect").addEventListener("change", () => {
    state.rateControl = $("#rateControlSelect").value;
    updateRateControlState();
    syncOutputSoon();
  });
  $("#audioSelect").addEventListener("change", () => {
    state.audioCodec = $("#audioSelect").value;
    updateAudioState();
  });
  $("#kcolorSelect").addEventListener("change", renderCliHint);

  for (const sel of ["#bitrateInput", "#audioBitrateInput"]) {
    $(sel).addEventListener("change", syncOutputSoon);
  }

  $("#previewButton").addEventListener("click", startPreview);
  $("#playButton").addEventListener("click", togglePlay);

  const seekBar = $("#seekBar");
  seekBar.addEventListener("pointerdown", () => {
    state.seeking = true;
  });
  seekBar.addEventListener("input", () => {
    state.seeking = true;
    seekTo(Number(seekBar.value));
  });
  seekBar.addEventListener("change", () => {
    state.seeking = false;
    seekTo(Number(seekBar.value));
  });
  seekBar.addEventListener("pointerup", () => {
    state.seeking = false;
  });

  $$("#modeSwitch button").forEach((button) => {
    button.addEventListener("click", () => setPlayerMode(button.dataset.mode));
  });
  $("#sideButton").addEventListener("click", () => setPlayerMode(state.playerMode === "side" ? "processed" : "side"));
  $("#dividerButton").addEventListener("click", () => setPlayerMode(state.playerMode === "divider" ? "processed" : "divider"));

  const divider = $("#dividerLine");
  divider.addEventListener("pointerdown", (event) => {
    if (state.playerMode !== "divider") return;
    state.draggingDivider = true;
    event.preventDefault();
  });
  window.addEventListener("pointermove", (event) => {
    if (!state.draggingDivider) return;
    const rect = $("#playerStage").getBoundingClientRect();
    if (rect.width > 0) setDivider(((event.clientX - rect.left) / rect.width) * 100);
  });
  window.addEventListener("pointerup", () => {
    state.draggingDivider = false;
  });

  for (const id of ["#videoOriginal", "#videoProcessed"]) {
    const video = $(id);
    video.addEventListener("play", updatePlayButton);
    video.addEventListener("pause", updatePlayButton);
    video.addEventListener("ended", updatePlayButton);
    video.addEventListener("loadedmetadata", () => {
      updatePlayerModeUI();
      updateTimeline();
    });
    video.addEventListener("error", updatePlayerModeUI);
  }
  $("#videoOriginal").addEventListener("ended", () => {
    const original = $("#videoOriginal");
    if (state.playerMode === "original") {
      original.currentTime = 0;
      original.play().catch(() => {});
    }
    updatePlayButton();
  });
  $("#videoProcessed").addEventListener("ended", () => {
    const original = $("#videoOriginal");
    const processed = $("#videoProcessed");
    processed.currentTime = 0;
    original.currentTime = state.preview ? state.preview.start : 0;
    Promise.all([processed.play(), original.play()]).catch(() => {});
    updatePlayButton();
  });

  $("#startButton").addEventListener("click", startJob);
  $("#cancelButton").addEventListener("click", cancelJob);
  $("#revealButton").addEventListener("click", revealOutput);
  $("#logButton").addEventListener("click", toggleLogs);

  document.addEventListener("pointerover", (event) => {
    const target = event.target.closest("[data-tooltip-key]");
    if (target) showTooltip(target);
  });
  document.addEventListener("pointerout", (event) => {
    const target = event.target.closest("[data-tooltip-key]");
    if (!target) return;
    if (event.relatedTarget && target.contains(event.relatedTarget)) return;
    hideTooltip();
  });
  window.addEventListener("scroll", hideTooltip, true);
  window.addEventListener("resize", hideTooltip);
  window.addEventListener("blur", hideTooltip);

  for (const ev of ["dragenter", "dragover"]) {
    window.addEventListener(ev, (e) => e.preventDefault());
  }
  window.addEventListener("drop", (e) => {
    e.preventDefault();
    toast(t("toast.dropUnsupported"));
  });
}

async function init() {
  requestAnimationFrame(playerTick);
  bindEvents();
  syncProgressDots();
  new ResizeObserver(syncProgressDots).observe($("#progressTrack"));
  updateStartState();
  let ok = true;
  try {
    await loadConfig();
    const app = state.config && state.config.app;
    if (app && app.restoreVideo && app.state && app.state.input && !state.input) {
      await loadInput(app.state.input);
    }
  } catch (err) {
    ok = false;
    toast(err.message, true);
    setStatusKey("status.connecting");
  }
  document.body.classList.remove("booting");
  $("#boot").hidden = true;
  if (!ok) return;
  if (state.outputDir) $("#outputDirInput").value = state.outputDir;
  setStatusKey("status.ready");
  resetTaskLabels();
  renderCliHint();
}

init();
function mediaURL(path) {
  return `/api/media?path=${encodeURIComponent(path)}`;
}

function clamp(v, lo, hi) {
  return Math.max(lo, Math.min(hi, v));
}

function inputDuration() {
  const value = $("#videoOriginal").duration;
  if (Number.isFinite(value) && value > 0) return value;
  return state.input && state.input.duration > 0 ? state.input.duration : 0;
}

function processedDuration() {
  const value = $("#videoProcessed").duration;
  if (Number.isFinite(value) && value > 0) return value;
  return state.preview ? state.preview.outputDuration : 0;
}

function previewSourceSpan() {
  return state.preview ? state.preview.sourceDuration : 0;
}

function formatSeconds(t) {
  if (!Number.isFinite(t) || t < 0) return "0.0s";
  return `${t.toFixed(1)}s`;
}

function playerPlaying() {
  if (state.playerMode === "original") return !$("#videoOriginal").paused;
  return !!(state.preview && !$("#videoProcessed").paused);
}

function updatePlayButton() {
  const playing = playerPlaying();
  $("#playButton").querySelector("use").setAttribute("href", playing ? "#i-pause" : "#i-play");
  $("#playButton").title = playing ? t("player.pause") : t("player.play");
}

function currentSourceTime() {
  const original = $("#videoOriginal");
  if (state.playerMode === "original" || !state.preview) {
    return original.currentTime || 0;
  }
  const out = processedDuration();
  if (out <= 0) return state.preview.start;
  const ratio = clamp($("#videoProcessed").currentTime / out, 0, 1);
  return state.preview.start + ratio * previewSourceSpan();
}

function updateTimeline() {
  const originalMode = state.playerMode === "original";
  const dur = originalMode ? inputDuration() : processedDuration();
  const seek = $("#seekBar");
  seek.max = dur > 0 ? dur : 1;
  if (!state.seeking) {
    const value = originalMode ? $("#videoOriginal").currentTime || 0 : $("#videoProcessed").currentTime || 0;
    seek.value = clamp(value, 0, dur || 1);
  }
  const range = $("#timelineRange");
  if (state.preview && dur > 0) {
    range.hidden = false;
    if (originalMode) {
      const left = clamp(state.preview.start / dur, 0, 1) * 100;
      const width = clamp(previewSourceSpan() / dur, 0, 1) * 100;
      range.style.left = `${left}%`;
      range.style.width = `${Math.max(width, 0.5)}%`;
    } else {
      range.style.left = "0%";
      range.style.width = "100%";
    }
  } else {
    range.hidden = true;
  }
  const label = $("#previewRangeLabel");
  if (state.preview) {
    const start = state.preview.start;
    const end = start + previewSourceSpan();
    let text = `${state.preview.full ? t("player.output") : t("task.preview")} ${formatSeconds(start)}–${formatSeconds(end)}`;
    if (!originalMode) text += ` → ${formatSeconds(processedDuration())}`;
    label.textContent = text;
    label.title = text;
  } else {
    label.textContent = "";
    label.title = "";
  }
  $("#previewDuration").textContent = originalMode
    ? formatDuration(inputDuration())
    : `${t("player.output")} ${formatDuration(processedDuration())}`;
}

function updatePlayerModeUI() {
  const stage = $("#playerStage");
  const mode = state.playerMode;
  stage.classList.remove("mode-original", "mode-processed", "mode-side", "mode-divider");
  stage.classList.add(`mode-${mode}`);
  const hasPreview = !!state.preview && !!$("#videoProcessed").getAttribute("src");
  const activeMode = mode === "original" ? "original" : "processed";
  $$("#modeSwitch button").forEach((button) => {
    button.classList.toggle("active", button.dataset.mode === activeMode);
    if (button.dataset.mode === "processed") button.disabled = !hasPreview;
  });
  $("#sideButton").disabled = !hasPreview;
  $("#dividerButton").disabled = !hasPreview;
  $("#sideButton").classList.toggle("active", mode === "side");
  $("#dividerButton").classList.toggle("active", mode === "divider");
  const line = $("#dividerLine");
  line.hidden = mode !== "divider" || !hasPreview;
  const message = $("#playerMessage");
  const compareMode = hasPreview && (mode === "side" || mode === "divider");
  $("#cmpLeft").hidden = !compareMode;
  $("#cmpRight").hidden = !compareMode;
  if (mode !== "original" && !hasPreview) {
    message.textContent = t("player.noPreview");
    message.hidden = false;
  } else if ($("#videoOriginal").error && mode === "original") {
    message.textContent = t("player.browserError");
    message.hidden = false;
  } else if ($("#videoProcessed").error && mode !== "original") {
    message.textContent = t("player.browserError");
    message.hidden = false;
  } else {
    message.hidden = true;
  }
  updateTimeline();
  updatePlayButton();
}

function setPlayerMode(mode) {
  if (mode !== "original" && !state.preview) return;
  const source = currentSourceTime();
  const original = $("#videoOriginal");
  const processed = $("#videoProcessed");
  const prevMode = state.playerMode;
  const keepPlaying = prevMode !== "original" ? !processed.paused : !original.paused;
  state.playerMode = mode;
  if (mode === "original") {
    original.pause();
    processed.pause();
    original.playbackRate = 1;
    const dur = inputDuration();
    if (dur > 0) original.currentTime = clamp(source, 0, dur);
    updatePlayerModeUI();
    return;
  }
  if (prevMode !== "original") {
    updatePlayerModeUI();
    return;
  }
  const out = processedDuration();
  const span = previewSourceSpan();
  processed.playbackRate = 1;
  original.playbackRate = out > 0 && span > 0 ? span / out : 1;
  const ratio = span > 0 ? clamp((source - state.preview.start) / span, 0, 1) : 0;
  if (out > 0) processed.currentTime = ratio * out;
  original.currentTime = state.preview.start + ratio * span;
  if (keepPlaying) {
    Promise.all([processed.play(), original.play()]).catch(() => {});
  }
  updatePlayerModeUI();
}

async function togglePlay() {
  if (!state.input) return;
  const original = $("#videoOriginal");
  const processed = $("#videoProcessed");
  if (state.playerMode === "original") {
    if (original.paused) {
      if (original.ended) original.currentTime = 0;
      try {
        await original.play();
      } catch {
        /* ignore autoplay rejection */
      }
    } else {
      original.pause();
    }
    return;
  }
  if (!state.preview) return;
  if (processed.paused) {
    const out = processedDuration();
    if (processed.ended || (out > 0 && processed.currentTime >= out - 0.02)) seekTo(0);
    const span = previewSourceSpan();
    original.currentTime = state.preview.start + (out > 0 ? (processed.currentTime / out) * span : 0);
    original.playbackRate = out > 0 && span > 0 ? span / out : 1;
    try {
      await Promise.all([processed.play(), original.play()]);
    } catch {
      /* ignore autoplay rejection */
    }
  } else {
    processed.pause();
    original.pause();
  }
}

function seekTo(t) {
  const original = $("#videoOriginal");
  const processed = $("#videoProcessed");
  if (state.playerMode === "original") {
    const dur = inputDuration();
    if (dur > 0) original.currentTime = clamp(t, 0, dur);
  } else if (state.preview) {
    const out = processedDuration();
    if (out <= 0) return;
    const ct = clamp(t, 0, out);
    processed.currentTime = ct;
    const span = previewSourceSpan();
    original.currentTime = state.preview.start + (ct / out) * span;
    original.playbackRate = span > 0 && out > 0 ? span / out : 1;
  }
  updateTimeline();
}

function setDivider(pct) {
  const value = clamp(pct, 0, 100);
  state.divider = value;
  $("#playerStage").style.setProperty("--divider", `${value}%`);
}

function playerTick() {
  if (state.input) {
    if (state.playerMode === "original") {
      if (!state.seeking) {
        const dur = inputDuration() || 1;
        $("#seekBar").max = dur;
        $("#seekBar").value = clamp($("#videoOriginal").currentTime || 0, 0, dur);
      }
    } else if (state.preview) {
      const out = processedDuration() || 1;
      const processed = $("#videoProcessed");
      const original = $("#videoOriginal");
      if (!state.seeking) {
        $("#seekBar").max = out;
        $("#seekBar").value = clamp(processed.currentTime || 0, 0, out);
      }
      const span = previewSourceSpan();
      const target = state.preview.start + (out > 0 ? clamp(processed.currentTime / out, 0, 1) * span : 0);
      if (!original.seeking && Math.abs((original.currentTime || 0) - target) > 0.25) {
        original.currentTime = target;
      }
      if (processed.ended) {
        processed.pause();
        original.pause();
      }
    }
    updatePlayButton();
  }
  requestAnimationFrame(playerTick);
}

function loadPreview(job) {
  state.preview = {
    path: job.output,
    start: job.previewStart || 0,
    sourceDuration: job.previewSourceDuration || 0,
    outputDuration: job.previewDuration || 0,
    full: !!job.full,
  };
  const processed = $("#videoProcessed");
  processed.src = mediaURL(job.output);
  processed.load();
  processed.addEventListener(
    "loadedmetadata",
    () => {
      if (!state.preview || state.preview.path !== job.output) return;
      state.preview.outputDuration = processed.duration || state.preview.outputDuration;
      updatePlayerModeUI();
      updateTimeline();
      if (state.playerMode !== "original") {
        const out = processedDuration();
        const span = previewSourceSpan();
        const original = $("#videoOriginal");
        original.playbackRate = out > 0 && span > 0 ? span / out : 1;
        original.currentTime = state.preview.start;
        processed.currentTime = 0;
        Promise.all([processed.play(), original.play()]).catch(() => {});
      }
    },
    { once: true }
  );
  const original = $("#videoOriginal");
  original.pause();
  original.currentTime = state.preview.start;
  state.playerMode = state.mode === "interp" ? "side" : "divider";
  setDivider(50);
  updatePlayerModeUI();
  updateTimeline();
}

function startPreview() {
  if (!state.input || state.busy) return;
  const duration = inputDuration();
  if (duration <= 0) {
    toast(t("toast.previewShort"), true);
    return;
  }
  const source = currentSourceTime();
  const req = buildRequest();
  req.preview = true;
  req.previewStart = clamp(source, 0, Math.max(0, duration - 0.02));
  submitJob(req, true);
}

function updatePreviewButton() {
  const button = $("#previewButton");
  button.disabled = !state.input || state.busy;
  button.hidden = state.busy;
  button.textContent = state.busy && state.job && state.job.preview ? t("task.previewing") : t("task.preview");
}
