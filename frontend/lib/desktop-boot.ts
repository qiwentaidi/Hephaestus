// 在 React 水合之前就给桌面端主窗口打上标记：首帧先遮住主界面，
// 避免冷启动时先闪一下仪表盘再出现模板索引加载页。
// 只判断“是否桌面 webview”，不依赖 localStorage 里的登录态：登录状态可能
// 由 DesktopStorageBridge 在水合后才同步，过早判断会漏掉首帧遮罩。
export const DESKTOP_BOOT_CLASS_SCRIPT = `(function(){try{
var search = window.location.search || "";
var hasDesktopToken = document.cookie.indexOf("hephaestus_desktop_api_token=") >= 0
  || search.indexOf("desktop_api_token=") >= 0
  || Boolean(window.sessionStorage.getItem("hephaestus:desktop-api-token"));
if (hasDesktopToken) {
  document.documentElement.classList.add("heph-desktop");
}
}catch(e){}})();`
