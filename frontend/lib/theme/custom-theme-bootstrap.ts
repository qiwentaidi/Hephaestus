export const CUSTOM_THEME_CSS_STORAGE_KEY = 'hephaestus:appearance:custom-theme-css'

/**
 * Runs before hydration/paint so a cached custom theme does not flash the
 * default theme on reload. Keep this dependency-free: it is inlined into the
 * document body.
 */
export const CUSTOM_THEME_BOOTSTRAP_SCRIPT = `(function(){try{var css=window.localStorage.getItem(${JSON.stringify(CUSTOM_THEME_CSS_STORAGE_KEY)});if(!css){return;}var style=document.getElementById('hephaestus-custom-theme');if(!style){style=document.createElement('style');style.id='hephaestus-custom-theme';document.head.appendChild(style);}style.textContent=css;}catch(error){}})();`
