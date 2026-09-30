//! The desktop app runs the Pimpo server as a sidecar on a free loopback
//! port and shows its web UI. Closing the window keeps the agent running in
//! the menu bar; only Quit stops it. It can instead open a Pimpo running
//! elsewhere (a home server, a VPS), with the same link the phone pairs
//! with; the local server then stays off, so one bot and one set of
//! routines never run twice. On mobile there is no sidecar: the shell page
//! always pairs with a Pimpo running elsewhere.

#[cfg(desktop)]
mod desktop {
    use std::net::{SocketAddr, TcpListener, TcpStream};
    use std::sync::Mutex;
    use std::time::{Duration, Instant};

    use tauri::menu::{CheckMenuItem, Menu, MenuItem, PredefinedMenuItem};
    use tauri::tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent};
    use tauri::webview::NewWindowResponse;
    use tauri::{AppHandle, Manager, Url, WebviewUrl, WebviewWindowBuilder, WindowEvent};
    use tauri_plugin_opener::OpenerExt;
    use tauri_plugin_autostart::ManagerExt as _;
    use tauri_plugin_shell::process::{CommandChild, CommandEvent};
    use tauri_plugin_shell::ShellExt;
    use tauri_plugin_updater::UpdaterExt;

    pub struct Server(pub Mutex<Option<CommandChild>>);

    /// Shell is the address of the app's own page, to come back to it from
    /// a remote Pimpo.
    pub struct Shell(pub Mutex<Option<Url>>);

    /// Local is the address of this computer's Pimpo once it answers.
    pub struct Local(pub Mutex<Option<String>>);

    /// Token signs the desktop app's own calls to this computer's Pimpo.
    pub struct Token(pub Mutex<Option<String>>);

    /// Lang is the language of the app's own menus: the system's at first,
    /// then the one chosen in Pimpo's settings.
    pub struct Lang(pub Mutex<&'static str>);

    /// TrayItems are the menu's items, to put them in another language.
    pub struct TrayItems(Vec<(&'static str, MenuItemKind)>);

    enum MenuItemKind {
        Plain(MenuItem<tauri::Wry>),
        Check(CheckMenuItem<tauri::Wry>),
    }

    const LANGS: [&str; 10] = ["pt", "en", "es", "fr", "de", "it", "ja", "zh", "ko", "ru"];

    /// WORDS are the menu's and the app's own messages, in LANGS order.
    const WORDS: &[(&str, [&str; 10])] = &[
        ("open", ["Abrir o Pimpo", "Open Pimpo", "Abrir Pimpo", "Ouvrir Pimpo", "Pimpo öffnen", "Apri Pimpo", "Pimpo を開く", "打开 Pimpo", "Pimpo 열기", "Открыть Pimpo"]),
        ("remote", ["Conectar a outro Pimpo…", "Connect to another Pimpo…", "Conectar a otro Pimpo…", "Se connecter à un autre Pimpo…", "Mit einem anderen Pimpo verbinden…", "Collegati a un altro Pimpo…", "別の Pimpo に接続…", "连接到另一个 Pimpo…", "다른 Pimpo에 연결…", "Подключиться к другому Pimpo…"]),
        ("local", ["Usar o Pimpo deste computador", "Use this computer's Pimpo", "Usar el Pimpo de esta computadora", "Utiliser le Pimpo de cet ordinateur", "Pimpo auf diesem Computer verwenden", "Usa il Pimpo di questo computer", "このパソコンの Pimpo を使う", "使用这台电脑上的 Pimpo", "이 컴퓨터의 Pimpo 사용", "Использовать Pimpo на этом компьютере"]),
        ("login", ["Abrir ao iniciar o computador", "Open at login", "Abrir al iniciar sesión", "Ouvrir à l’ouverture de session", "Beim Anmelden öffnen", "Apri all’accesso", "ログイン時に開く", "登录时打开", "로그인 시 열기", "Открывать при входе"]),
        ("mascot", ["Pimpo na área de trabalho", "Pimpo on the desktop", "Pimpo en el escritorio", "Pimpo sur le bureau", "Pimpo auf dem Schreibtisch", "Pimpo sulla scrivania", "デスクトップの Pimpo", "桌面上的 Pimpo", "데스크톱의 Pimpo", "Pimpo на рабочем столе"]),
        ("update", ["Procurar atualizações", "Check for updates", "Buscar actualizaciones", "Rechercher des mises à jour", "Nach Updates suchen", "Cerca aggiornamenti", "アップデートを確認", "检查更新", "업데이트 확인", "Проверить обновления"]),
        ("install", ["Instalar a versão {} e reiniciar", "Install version {} and restart", "Instalar la versión {} y reiniciar", "Installer la version {} et redémarrer", "Version {} installieren und neu starten", "Installa la versione {} e riavvia", "バージョン {} をインストールして再起動", "安装版本 {} 并重启", "버전 {} 설치 후 다시 시작", "Установить версию {} и перезапустить"]),
        ("quit", ["Sair do Pimpo", "Quit Pimpo", "Salir de Pimpo", "Quitter Pimpo", "Pimpo beenden", "Esci da Pimpo", "Pimpo を終了", "退出 Pimpo", "Pimpo 종료", "Выйти из Pimpo"]),
        ("notLink", ["Isso não parece um link.", "That doesn’t look like a link.", "Eso no parece un enlace.", "Cela ne ressemble pas à un lien.", "Das sieht nicht nach einem Link aus.", "Non sembra un link.", "リンクではないようです。", "这看起来不像链接。", "링크가 아닌 것 같아요.", "Это не похоже на ссылку."]),
        ("https", ["Use https: o token não pode viajar em aberto.", "Use https: the token can’t travel in the clear.", "Usa https: el token no puede viajar sin cifrar.", "Utilisez https : le token ne peut pas circuler en clair.", "Nutze https: Das Token darf nicht unverschlüsselt übertragen werden.", "Usa https: il token non può viaggiare in chiaro.", "https を使ってください。token を暗号化なしで送ることはできません。", "请使用 https：token 不能明文传输。", "https를 사용하세요. token은 암호화 없이 보낼 수 없어요.", "Используйте https: token нельзя передавать в открытом виде."]),
        ("noToken", ["Falta o token no link.", "The link is missing its token.", "Al enlace le falta el token.", "Il manque le token dans le lien.", "Im Link fehlt das Token.", "Nel link manca il token.", "リンクに token がありません。", "链接里缺少 token。", "링크에 token이 없어요.", "В ссылке нет token."]),
        ("badHome", ["Endereço de casa inválido.", "That home address is not valid.", "La dirección de casa no es válida.", "L’adresse de la maison n’est pas valide.", "Die Heimadresse ist ungültig.", "L’indirizzo di casa non è valido.", "自宅のアドレスが正しくありません。", "家庭地址无效。", "집 주소가 올바르지 않아요.", "Домашний адрес неверен."]),
        ("homeLocal", ["O endereço de casa precisa ser da rede local.", "The home address must be on the home network.", "La dirección de casa debe ser de la red local.", "L’adresse de la maison doit être sur le réseau local.", "Die Heimadresse muss im Heimnetz liegen.", "L’indirizzo di casa deve essere sulla rete locale.", "自宅のアドレスはホームネットワーク内である必要があります。", "家庭地址必须在家庭网络中。", "집 주소는 홈 네트워크에 있어야 해요.", "Домашний адрес должен быть в домашней сети."]),
        ("noConfig", ["Sem pasta de configuração.", "There is no settings folder.", "No hay carpeta de configuración.", "Il n’y a pas de dossier de réglages.", "Es gibt keinen Einstellungsordner.", "Non c’è una cartella delle impostazioni.", "設定フォルダーがありません。", "没有设置文件夹。", "설정 폴더가 없어요.", "Нет папки настроек."]),
    ];

    /// language picks one of LANGS from a locale such as pt_BR.UTF-8 or
    /// zh-Hans; anything else is English.
    pub fn language(locale: &str) -> &'static str {
        let code = locale.trim().to_lowercase();
        let code = code.split(['-', '_', '.']).next().unwrap_or("");
        LANGS.iter().find(|l| **l == code).copied().unwrap_or("en")
    }

    /// word is a message in a language.
    pub fn word(lang: &str, key: &str) -> &'static str {
        let i = LANGS.iter().position(|l| *l == lang).unwrap_or(1);
        WORDS.iter().find(|(k, _)| *k == key).map(|(_, w)| w[i]).unwrap_or("")
    }

    fn tr(app: &AppHandle, key: &str) -> &'static str {
        let lang = app.try_state::<Lang>().map(|l| *l.0.lock().unwrap()).unwrap_or_else(|| language(&system_locale()));
        word(lang, key)
    }

    /// system_locale is the language the computer is set to.
    fn system_locale() -> String {
        for v in ["LC_ALL", "LC_MESSAGES", "LANG"] {
            if let Ok(s) = std::env::var(v) {
                if !s.is_empty() && s != "C" && s != "POSIX" {
                    return s;
                }
            }
        }
        #[cfg(target_os = "macos")]
        if let Ok(o) = std::process::Command::new("defaults").args(["read", "-g", "AppleLocale"]).output() {
            let s = String::from_utf8_lossy(&o.stdout).trim().to_string();
            if !s.is_empty() {
                return s;
            }
        }
        #[cfg(windows)]
        {
            use std::os::windows::process::CommandExt;
            if let Ok(o) = std::process::Command::new("powershell").args(["-NoProfile", "-Command", "(Get-Culture).Name"]).creation_flags(0x0800_0000).output() {
                let s = String::from_utf8_lossy(&o.stdout).trim().to_string();
                if !s.is_empty() {
                    return s;
                }
            }
        }
        "en".into()
    }

    /// set_language puts the menus in a language, when it changed.
    fn set_language(app: &AppHandle, lang: &'static str) {
        {
            let state = app.state::<Lang>();
            let mut cur = state.0.lock().unwrap();
            if *cur == lang {
                return;
            }
            *cur = lang;
        }
        if let Some(items) = app.try_state::<TrayItems>() {
            for (key, item) in &items.0 {
                let _ = match item {
                    MenuItemKind::Plain(i) => i.set_text(word(lang, key)),
                    MenuItemKind::Check(i) => i.set_text(word(lang, key)),
                };
            }
        }
        broadcast_update(app);
    }

    /// follow_language keeps the menus in the language chosen in Pimpo's
    /// settings, checking once a minute.
    fn follow_language(app: AppHandle) {
        std::thread::spawn(move || loop {
            if let Some(body) = local_api(&app, "GET", "/api/settings", None) {
                if let Ok(v) = serde_json::from_str::<serde_json::Value>(&body) {
                    if let Some(l) = v.get("locale").and_then(|l| l.as_str()) {
                        set_language(&app, language(l));
                    }
                }
            }
            std::thread::sleep(Duration::from_secs(60));
        });
    }

    fn mascot_file(app: &AppHandle) -> Option<std::path::PathBuf> {
        app.path().app_config_dir().ok().map(|d| d.join("mascot-on"))
    }

    fn mascot_wanted(app: &AppHandle) -> bool {
        mascot_file(app).map(|f| f.exists()).unwrap_or(false)
    }

    /// is_local reports whether a URL is this computer's Pimpo, the only
    /// page allowed to switch the floating cat.
    fn is_local(app: &AppHandle, u: &Url) -> bool {
        let local = app.try_state::<Local>().and_then(|l| l.0.lock().unwrap().clone());
        local.is_some_and(|base| same_origin(&base, u))
    }

    /// same_origin reports whether a URL has exactly the scheme, host and
    /// port of base, and no user name or password (so a link such as
    /// http://127.0.0.1:PORT@elsewhere, or another port starting with the
    /// same digits, does not pass).
    pub fn same_origin(base: &str, u: &Url) -> bool {
        let Ok(b) = Url::parse(base) else { return false };
        u.username().is_empty()
            && u.password().is_none()
            && u.scheme() == b.scheme()
            && u.host_str().is_some()
            && u.host_str() == b.host_str()
            && u.port_or_known_default() == b.port_or_known_default()
    }

    /// set_mascot keeps the choice, shows or hides the cat, and tells the
    /// app's pages and the menu bar. Turning it off lets the cat wave and
    /// trot off the screen first; the window closes when it has left, or
    /// after a few seconds in any case.
    pub fn set_mascot(app: &AppHandle, on: bool) {
        keep_mascot(app, on);
        if on {
            mascot(app, true);
            return;
        }
        if let Some(w) = app.get_webview_window("mascot") {
            let _ = w.eval("window.__pimpoGoodbye ? window.__pimpoGoodbye() : window.location.assign('/desktop/mascot?on=0')");
            let handle = app.clone();
            std::thread::spawn(move || {
                std::thread::sleep(Duration::from_secs(6));
                mascot(&handle, false);
            });
        }
    }

    /// keep_mascot records the choice and shows it in the app and menu bar.
    fn keep_mascot(app: &AppHandle, on: bool) {
        if let Some(f) = mascot_file(app) {
            let _ = if on {
                f.parent().map(std::fs::create_dir_all);
                std::fs::write(&f, b"on")
            } else {
                std::fs::remove_file(&f)
            };
        }
        if let Some(w) = app.get_webview_window("main") {
            sync_mascot(app, &w);
        }
        if let Some(item) = app.try_state::<MascotItem>() {
            let _ = item.0.set_checked(on);
        }
    }

    /// sync_mascot tells a page whether the floating cat is on, since the
    /// page's own storage changes with the port on every start.
    fn sync_mascot(app: &AppHandle, w: &tauri::WebviewWindow) {
        let on = if mascot_wanted(app) { "on" } else { "off" };
        let _ = w.eval(format!("try {{ localStorage.setItem('pimpo.mascot', '{on}'); window.dispatchEvent(new Event('pimpo:mascot')) }} catch (e) {{}}"));
    }

    pub struct MascotItem(pub CheckMenuItem<tauri::Wry>);

    fn corner_file(app: &AppHandle) -> Option<std::path::PathBuf> {
        app.path().app_config_dir().ok().map(|d| d.join("mascot-corner"))
    }

    /// mascot_corner is the floating cat's bottom-right corner, in logical
    /// points, where the owner last left it.
    fn mascot_corner(app: &AppHandle) -> Option<(f64, f64)> {
        let text = std::fs::read_to_string(corner_file(app)?).ok()?;
        let (x, y) = text.trim().split_once(',')?;
        Some((x.parse().ok()?, y.parse().ok()?))
    }

    /// on_screen reports whether a window at x, y (logical points) fits
    /// entirely inside one of the monitors.
    fn on_screen(app: &AppHandle, x: f64, y: f64, w: f64, h: f64) -> bool {
        app.available_monitors().unwrap_or_default().iter().any(|m| {
            let scale = m.scale_factor();
            let p = m.position().to_logical::<f64>(scale);
            let s = m.size().to_logical::<f64>(scale);
            x >= p.x && y >= p.y && x + w <= p.x + s.width && y + h <= p.y + s.height
        })
    }

    /// remember_corner keeps where the cat is after it moves. It only counts
    /// when the window is just the cat: while it grows or shrinks for a
    /// bubble or a game it moves and resizes in two steps, and the corner in
    /// between is not where the cat is.
    pub fn remember_corner(window: &tauri::Window) {
        let (Ok(pos), Ok(size), Ok(scale)) = (window.outer_position(), window.outer_size(), window.scale_factor()) else { return };
        let (lw, lh) = (size.width as f64 / scale, size.height as f64 / scale);
        if (lw - 96.0).abs() > 2.0 || (lh - 120.0).abs() > 2.0 {
            return;
        }
        let right = (pos.x as f64 + size.width as f64) / scale;
        let bottom = (pos.y as f64 + size.height as f64) / scale;
        if let Some(f) = corner_file(window.app_handle()) {
            let _ = std::fs::write(f, format!("{right:.0},{bottom:.0}"));
        }
    }

    /// mascot shows or hides the floating Pimpo: a small transparent window
    /// above everything, with only the cat. It grows while its bubble or
    /// menu is open, so the corner of the screen stays clickable. It shows
    /// this computer's Pimpo only.
    pub fn mascot(app: &AppHandle, on: bool) {
        if !on {
            if let Some(w) = app.get_webview_window("mascot") {
                let _ = w.close();
            }
            return;
        }
        if app.get_webview_window("mascot").is_some() || saved_remote(app).is_some() {
            return;
        }
        let Some(base) = app.try_state::<Local>().and_then(|l| l.0.lock().unwrap().clone()) else { return };
        let Ok(url) = Url::parse(&format!("{base}/mascot")) else { return };
        let (w, h) = (96.0, 120.0);
        let mut x = 40.0;
        let mut y = 40.0;
        if let Ok(Some(m)) = app.primary_monitor() {
            let size = m.size().to_logical::<f64>(m.scale_factor());
            x = size.width - w - 24.0;
            y = size.height - h - 96.0;
        }
        // Back where the owner left it: its bottom-right corner is kept, since
        // the window grows up and to the left for the bubble and the games.
        // A corner no monitor contains (say, one since unplugged) is dropped.
        if let Some((right, bottom)) = mascot_corner(app).filter(|&(r, b)| on_screen(app, r - w, b - h, w, h)) {
            x = right - w;
            y = bottom - h;
        }
        let main = app.clone();
        let open_base = base.clone();
        let _ = WebviewWindowBuilder::new(app, "mascot", WebviewUrl::External(url))
            .title("Pimpo")
            .transparent(true)
            .decorations(false)
            .shadow(false)
            .always_on_top(true)
            .skip_taskbar(true)
            .resizable(false)
            .visible_on_all_workspaces(true)
            .inner_size(w, h)
            .position(x, y)
            // The cat's links open in the main window instead of its own.
            .on_navigation(move |u| {
                // The cat's window only ever shows this computer's Pimpo.
                if !same_origin(&open_base, u) {
                    return false;
                }
                // The cat said goodbye (from its own menu, or when turned off).
                if u.path() == "/desktop/mascot" {
                    keep_mascot(&main, false);
                    mascot(&main, false);
                    return false;
                }
                if u.path() != "/open" {
                    return true;
                }
                let path = u.query_pairs().find(|(k, _)| k == "path").map(|(_, v)| v.to_string()).unwrap_or_else(|| "/".into());
                if path.starts_with('/') {
                    if let (Ok(target), Some(w)) = (Url::parse(&format!("{open_base}{path}")), main.get_webview_window("main")) {
                        let _ = w.navigate(target);
                    }
                }
                show(&main);
                false
            })
            .build();
    }

    fn remote_file(app: &AppHandle) -> Option<std::path::PathBuf> {
        app.path().app_config_dir().ok().map(|d| d.join("remote.txt"))
    }

    /// The remote Pimpo chosen, as its link and optional home address.
    pub fn saved_remote(app: &AppHandle) -> Option<(String, String)> {
        let text = std::fs::read_to_string(remote_file(app)?).ok()?;
        let mut lines = text.lines();
        let link = lines.next()?.trim().to_string();
        let home = lines.next().unwrap_or("").trim().to_string();
        (!link.is_empty()).then_some((link, home))
    }

    fn private_host(host: &str) -> bool {
        let octets: Vec<u8> = host.split('.').filter_map(|p| p.parse().ok()).collect();
        if host == "localhost" {
            return true;
        }
        if octets.len() != 4 {
            return false;
        }
        matches!(octets[..], [127, ..] | [10, ..] | [192, 168, ..]) || (octets[0] == 172 && (16..=31).contains(&octets[1])) || (octets[0] == 100 && (64..=127).contains(&octets[1]))
    }

    /// check_link accepts a pairing link: https, or http only inside the
    /// home network or Tailscale, and with its token.
    pub fn check_link(link: &str) -> Result<Url, String> {
        let url = Url::parse(link).map_err(|_| word(language(&system_locale()), "notLink").to_string())?;
        let host = url.host_str().unwrap_or("");
        match url.scheme() {
            "https" => {}
            "http" if private_host(host) => {}
            _ => return Err(word(language(&system_locale()), "https").into()),
        }
        if !url.query_pairs().any(|(k, v)| k == "token" && !v.is_empty()) {
            return Err(word(language(&system_locale()), "noToken").into());
        }
        Ok(url)
    }

    #[tauri::command]
    pub fn remote(app: AppHandle) -> Vec<String> {
        saved_remote(&app).map(|(l, h)| vec![l, h]).unwrap_or_default()
    }

    /// use_remote keeps the link and stops the local Pimpo.
    #[tauri::command]
    pub fn use_remote(app: AppHandle, link: String, home: String) -> Result<(), String> {
        check_link(&link)?;
        if !home.is_empty() {
            let h = Url::parse(&home).map_err(|_| tr(&app, "badHome").to_string())?;
            if h.scheme() != "http" || !private_host(h.host_str().unwrap_or("")) {
                return Err(tr(&app, "homeLocal").into());
            }
        }
        let file = remote_file(&app).ok_or(tr(&app, "noConfig"))?;
        if let Some(dir) = file.parent() {
            std::fs::create_dir_all(dir).map_err(|e| e.to_string())?;
        }
        std::fs::write(&file, format!("{link}\n{home}\n")).map_err(|e| e.to_string())?;
        mascot(&app, false);
        *app.state::<Local>().0.lock().unwrap() = None;
        stop(&app);
        Ok(())
    }

    /// use_local forgets the remote Pimpo and starts the one on this
    /// computer, which opens when it is ready.
    #[tauri::command]
    pub fn use_local(app: AppHandle) -> Result<(), String> {
        if let Some(file) = remote_file(&app) {
            let _ = std::fs::remove_file(file);
        }
        start_local(&app).map_err(|e| e.to_string())
    }

    /// pair shows the app's own page with the link form.
    pub fn pair(app: &AppHandle) {
        let shell = app.try_state::<Shell>().and_then(|s| s.0.lock().unwrap().clone());
        if let (Some(mut url), Some(w)) = (shell, app.get_webview_window("main")) {
            url.set_fragment(Some("pair"));
            let _ = w.navigate(url);
            show(app);
        }
    }

    // The window is built here rather than from the config so the page
    // knows it is in the desktop app (room for the window buttons, links sent
    // to the browser) and so new-window requests do not vanish.
    pub fn window(app: &AppHandle) -> tauri::Result<()> {
        let conf = app.config().app.windows.iter().find(|w| w.label == "main").expect("main window config").clone();
        let handle = app.clone();
        let platform = if cfg!(target_os = "macos") { "mac" } else { "on" };
        let nav = app.clone();
        let loaded = app.clone();
        let w = WebviewWindowBuilder::from_config(app, &conf)?
            .initialization_script(format!("window.__PIMPO_DESKTOP__ = {platform:?}"))
            // Ajustes switches the floating Pimpo by visiting /desktop/mascot,
            // so the page needs no native access.
            .on_navigation(move |u| {
                if !is_local(&nav, u) {
                    return true;
                }
                match u.path() {
                    "/desktop/mascot" => {
                        let on = u.query_pairs().any(|(k, v)| k == "on" && v == "1");
                        set_mascot(&nav, on);
                        false
                    }
                    // Updates are asked for the same way. The page can only
                    // ask: what gets installed is the signed update the app
                    // itself found.
                    "/desktop/update" => {
                        match u.query_pairs().find(|(k, _)| k == "do").map(|(_, v)| v.to_string()).as_deref() {
                            Some("install") => install_update(&nav),
                            Some("rollback") => rollback(&nav),
                            Some("beta") => {
                                set_beta(&nav, true);
                                check_updates(&nav, true);
                            }
                            Some("stable") => {
                                set_beta(&nav, false);
                                check_updates(&nav, true);
                            }
                            _ => check_updates(&nav, true),
                        }
                        false
                    }
                    _ => true,
                }
            })
            .on_page_load(move |w, p| {
                if matches!(p.event(), tauri::webview::PageLoadEvent::Finished) {
                    sync_mascot(&loaded, &w);
                    sync_update(&loaded, &w);
                }
            })
            .on_new_window(move |url, _| {
                if matches!(url.scheme(), "http" | "https" | "mailto") {
                    let _ = handle.opener().open_url(url.as_str(), None::<&str>);
                }
                NewWindowResponse::Deny
            })
            .build()?;
        app.manage(Shell(Mutex::new(w.url().ok())));
        app.manage(Local(Mutex::new(None)));
        app.manage(Server(Mutex::new(None)));
        app.manage(Token(Mutex::new(None)));
        app.manage(Lang(Mutex::new(language(&system_locale()))));
        app.manage(Updates::default());
        Ok(())
    }

    pub fn show(app: &AppHandle) {
        if let Some(w) = app.get_webview_window("main") {
            let _ = w.show();
            let _ = w.unminimize();
            let _ = w.set_focus();
        }
    }

    /// status shows a line on the app's own page: a known key ("stopped",
    /// "slow") in the page's language, or a raw line from the server.
    fn status(app: &AppHandle, text: &str) {
        status_with(app, text, "")
    }

    fn status_with(app: &AppHandle, key: &str, arg: &str) {
        if let Some(w) = app.get_webview_window("main") {
            let js = format!("window.__pimpoStatus && window.__pimpoStatus({key:?}, {arg:?})");
            let _ = w.eval(&js);
        }
    }

    fn free_port() -> std::io::Result<u16> {
        Ok(TcpListener::bind("127.0.0.1:0")?.local_addr()?.port())
    }

    fn token() -> String {
        let mut b = [0u8; 24];
        getrandom::fill(&mut b).expect("no randomness available");
        b.iter().map(|x| format!("{x:02x}")).collect()
    }

    /// start runs the local Pimpo unless a remote one was chosen; the shell
    /// page then connects to it.
    pub fn start(app: &AppHandle) -> Result<(), Box<dyn std::error::Error>> {
        if saved_remote(app).is_some() {
            return Ok(());
        }
        start_local(app)
    }

    pub fn start_local(app: &AppHandle) -> Result<(), Box<dyn std::error::Error>> {
        if app.state::<Server>().0.lock().unwrap().is_some() {
            return Ok(());
        }
        let port = free_port()?;
        let token = token();
        *app.state::<Token>().0.lock().unwrap() = Some(token.clone());
        let addr = format!("127.0.0.1:{port}");
        let (mut rx, child) = app
            .shell()
            .sidecar("pimpo")?
            .args(["serve", "--addr", &addr])
            .env("PIMPO_TOKEN", &token)
            .env("PIMPO_EXIT_WITH_PARENT", "1")
            .env("PIMPO_DESKTOP_NOTIFY", "1")
            .spawn()?;
        *app.state::<Server>().0.lock().unwrap() = Some(child);

        let handle = app.clone();
        tauri::async_runtime::spawn(async move {
            while let Some(ev) = rx.recv().await {
                match ev {
                    CommandEvent::Stderr(line) => {
                        let line = String::from_utf8_lossy(&line).trim().to_string();
                        if !line.is_empty() {
                            status(&handle, &line);
                        }
                    }
                    CommandEvent::Terminated(p) => {
                        // Stopped on purpose when switching to a remote Pimpo.
                        if saved_remote(&handle).is_some() {
                            return;
                        }
                        status_with(&handle, "stopped", &format!("{}", p.code.unwrap_or(-1)));
                        show(&handle);
                    }
                    _ => {}
                }
            }
        });

        let handle = app.clone();
        std::thread::spawn(move || {
            let target: SocketAddr = addr.parse().unwrap();
            let deadline = Instant::now() + Duration::from_secs(30);
            while Instant::now() < deadline {
                if TcpStream::connect_timeout(&target, Duration::from_millis(300)).is_ok() {
                    let url = Url::parse(&format!("http://{addr}/auth?token={token}")).unwrap();
                    if let Some(w) = handle.get_webview_window("main") {
                        let _ = w.navigate(url);
                    }
                    *handle.state::<Local>().0.lock().unwrap() = Some(format!("http://{addr}"));
                    follow_language(handle.clone());
                    if mascot_wanted(&handle) {
                        // After the main window has signed in, so the cat shares its session.
                        std::thread::sleep(Duration::from_millis(1500));
                        mascot(&handle, true);
                    }
                    return;
                }
                std::thread::sleep(Duration::from_millis(150));
            }
            status(&handle, "slow");
        });
        Ok(())
    }

    pub fn stop(app: &AppHandle) {
        if let Some(s) = app.try_state::<Server>() {
            if let Some(child) = s.0.lock().unwrap().take() {
                let _ = child.kill();
            }
        }
    }

    pub fn tray(app: &AppHandle) -> tauri::Result<()> {
        let open = MenuItem::with_id(app, "open", tr(app, "open"), true, None::<&str>)?;
        let remote = MenuItem::with_id(app, "remote", tr(app, "remote"), true, None::<&str>)?;
        let local = MenuItem::with_id(app, "local", tr(app, "local"), true, None::<&str>)?;
        let at_login = app.autolaunch().is_enabled().unwrap_or(false);
        let login = CheckMenuItem::with_id(app, "login", tr(app, "login"), true, at_login, None::<&str>)?;
        let cat = CheckMenuItem::with_id(app, "mascot", tr(app, "mascot"), true, mascot_wanted(app), None::<&str>)?;
        app.manage(MascotItem(cat.clone()));
        let update = MenuItem::with_id(app, "update", tr(app, "update"), true, None::<&str>)?;
        app.manage(UpdateItem(update.clone()));
        let quit = MenuItem::with_id(app, "quit", tr(app, "quit"), true, Some("CmdOrCtrl+Q"))?;
        app.manage(TrayItems(vec![
            ("open", MenuItemKind::Plain(open.clone())),
            ("remote", MenuItemKind::Plain(remote.clone())),
            ("local", MenuItemKind::Plain(local.clone())),
            ("login", MenuItemKind::Check(login.clone())),
            ("mascot", MenuItemKind::Check(cat.clone())),
            ("quit", MenuItemKind::Plain(quit.clone())),
        ]));
        let menu = Menu::with_items(app, &[&open, &cat, &login, &PredefinedMenuItem::separator(app)?, &remote, &local, &PredefinedMenuItem::separator(app)?, &update, &quit])?;
        TrayIconBuilder::with_id("pimpo")
            .icon(tauri::image::Image::from_bytes(include_bytes!("../icons/tray.png"))?)
            .icon_as_template(true)
            .tooltip("Pimpo")
            .menu(&menu)
            .show_menu_on_left_click(false)
            .on_menu_event(move |app, ev| match ev.id().as_ref() {
                "open" => show(app),
                "remote" => pair(app),
                "mascot" => set_mascot(app, !mascot_wanted(app)),
                "local" => {
                    if saved_remote(app).is_some() {
                        if let Some(url) = app.try_state::<Shell>().and_then(|s| s.0.lock().unwrap().clone()) {
                            if let Some(w) = app.get_webview_window("main") {
                                let _ = w.navigate(url);
                            }
                        }
                    }
                    let _ = use_local(app.clone());
                    show(app);
                }
                "update" => {
                    if app.state::<Updates>().found.lock().unwrap().is_some() {
                        install_update(app);
                    } else {
                        check_updates(app, true);
                        show(app);
                    }
                }
                "login" => {
                    let al = app.autolaunch();
                    let _ = if al.is_enabled().unwrap_or(false) { al.disable() } else { al.enable() };
                }
                "quit" => {
                    stop(app);
                    app.exit(0);
                }
                _ => {}
            })
            .on_tray_icon_event(|tray, ev| {
                if let TrayIconEvent::Click { button: MouseButton::Left, button_state: MouseButtonState::Up, .. } = ev {
                    show(tray.app_handle());
                }
            })
            .build(app)?;
        Ok(())
    }

    // Updates come from the project's releases, signed with its key (the
    // public half is in tauri.conf.json): stable from the latest release,
    // beta from the channel-beta release the release workflow keeps current.
    // Before installing, the version being left is remembered, so "go back"
    // can restore the snapshot Pimpo took of the data when the new version
    // started and reinstall the old one.

    const RELEASES: &str = "https://github.com/turbine-dev/pimpo/releases";

    #[derive(Default)]
    pub struct Updates {
        pub found: Mutex<Option<tauri_plugin_updater::Update>>,
        pub error: Mutex<String>,
        pub busy: Mutex<bool>,
    }

    pub struct UpdateItem(pub MenuItem<tauri::Wry>);

    fn data_file(app: &AppHandle, name: &str) -> Option<std::path::PathBuf> {
        app.path().app_data_dir().ok().map(|d| d.join(name))
    }

    fn beta(app: &AppHandle) -> bool {
        data_file(app, "update-beta").is_some_and(|p| p.exists())
    }

    fn set_beta(app: &AppHandle, on: bool) {
        if let Some(p) = data_file(app, "update-beta") {
            let _ = if on {
                p.parent().map(std::fs::create_dir_all);
                std::fs::write(p, "beta")
            } else {
                std::fs::remove_file(p)
            };
        }
    }

    fn previous(app: &AppHandle) -> Option<String> {
        data_file(app, "previous-version").and_then(|p| std::fs::read_to_string(p).ok()).map(|v| v.trim().to_string()).filter(|v| !v.is_empty())
    }

    pub fn update_endpoint(beta: bool) -> Url {
        let u = if beta { format!("{RELEASES}/download/channel-beta/latest.json") } else { format!("{RELEASES}/latest/download/latest.json") };
        Url::parse(&u).unwrap()
    }

    pub fn version_endpoint(version: &str) -> Url {
        Url::parse(&format!("{RELEASES}/download/v{version}/latest.json")).unwrap()
    }

    fn update_state(app: &AppHandle) -> String {
        let u = app.state::<Updates>();
        let found = u.found.lock().unwrap();
        serde_json::json!({
            "current": app.package_info().version.to_string(),
            "found": found.as_ref().map(|f| f.version.clone()),
            "notes": found.as_ref().and_then(|f| f.body.clone()),
            "beta": beta(app),
            "previous": previous(app),
            "busy": *u.busy.lock().unwrap(),
            "error": u.error.lock().unwrap().clone(),
        })
        .to_string()
    }

    /// sync_update tells the page what the app knows about updates, the
    /// same way as the floating Pimpo's switch.
    fn sync_update(app: &AppHandle, w: &tauri::WebviewWindow) {
        let state = update_state(app);
        let _ = w.eval(format!("try {{ localStorage.setItem('pimpo.update', {state:?}); window.dispatchEvent(new Event('pimpo:update')) }} catch (e) {{}}"));
    }

    fn broadcast_update(app: &AppHandle) {
        if let Some(w) = app.get_webview_window("main") {
            sync_update(app, &w);
        }
        if let Some(item) = app.try_state::<UpdateItem>() {
            let text = match app.state::<Updates>().found.lock().unwrap().as_ref() {
                Some(f) => tr(app, "install").replace("{}", &f.version.to_string()),
                None => tr(app, "update").to_string(),
            };
            let _ = item.0.set_text(text);
        }
    }

    fn set_error(app: &AppHandle, e: impl std::fmt::Display) {
        *app.state::<Updates>().error.lock().unwrap() = e.to_string();
        *app.state::<Updates>().busy.lock().unwrap() = false;
        broadcast_update(app);
    }

    /// A Flatpak is updated by Flathub, not by the app.
    fn managed_elsewhere() -> bool {
        std::env::var_os("FLATPAK_ID").is_some()
    }

    pub fn check_updates(app: &AppHandle, manual: bool) {
        if managed_elsewhere() {
            if manual {
                set_error(app, "this Pimpo is updated by Flathub");
            }
            return;
        }
        let app = app.clone();
        tauri::async_runtime::spawn(async move {
            {
                let u = app.state::<Updates>();
                if *u.busy.lock().unwrap() {
                    return;
                }
                *u.busy.lock().unwrap() = manual;
                u.error.lock().unwrap().clear();
            }
            if manual {
                broadcast_update(&app);
            }
            let updater = match app.updater_builder().endpoints(vec![update_endpoint(beta(&app))]).and_then(|b| b.build()) {
                Ok(u) => u,
                Err(e) => return set_error(&app, e),
            };
            match updater.check().await {
                Ok(found) => *app.state::<Updates>().found.lock().unwrap() = found,
                // A check in the background that fails says nothing.
                Err(e) if manual => return set_error(&app, e),
                Err(_) => {}
            }
            *app.state::<Updates>().busy.lock().unwrap() = false;
            broadcast_update(&app);
        });
    }

    /// check_periodically looks for updates at start and every six hours.
    pub fn check_periodically(app: &AppHandle) {
        let app = app.clone();
        std::thread::spawn(move || loop {
            std::thread::sleep(Duration::from_secs(60));
            check_updates(&app, false);
            std::thread::sleep(Duration::from_secs(6 * 3600 - 60));
        });
    }

    pub fn install_update(app: &AppHandle) {
        let app = app.clone();
        tauri::async_runtime::spawn(async move {
            let Some(update) = app.state::<Updates>().found.lock().unwrap().take() else {
                return check_updates(&app, true);
            };
            *app.state::<Updates>().busy.lock().unwrap() = true;
            broadcast_update(&app);
            if let Some(p) = data_file(&app, "previous-version") {
                p.parent().map(std::fs::create_dir_all);
                let _ = std::fs::write(p, app.package_info().version.to_string());
            }
            match update.download_and_install(|_, _| {}, || {}).await {
                Ok(()) => {
                    stop(&app);
                    app.restart();
                }
                Err(e) => set_error(&app, e),
            }
        });
    }

    /// rollback goes back to the version installed before the last update:
    /// Pimpo restores the snapshot the new version took of the data when
    /// it first started (newer data may not suit an older Pimpo), then the
    /// old version is installed and started.
    pub fn rollback(app: &AppHandle) {
        let app = app.clone();
        tauri::async_runtime::spawn(async move {
            let Some(prev) = previous(&app) else {
                return set_error(&app, "there is no earlier version to go back to");
            };
            *app.state::<Updates>().busy.lock().unwrap() = true;
            broadcast_update(&app);
            let current = app.package_info().version.to_string();
            if let Some(list) = local_api(&app, "GET", "/api/snapshots", None) {
                let names: Vec<String> = serde_json::from_str::<serde_json::Value>(&list)
                    .ok()
                    .and_then(|v| v["snapshots"].as_array().cloned())
                    .unwrap_or_default()
                    .iter()
                    .filter_map(|s| s["name"].as_str().map(str::to_string))
                    .collect();
                if let Some(name) = names.iter().filter(|n| n.contains(&format!("before-{current}"))).max() {
                    let body = serde_json::json!({ "name": name }).to_string();
                    if local_api(&app, "POST", "/api/snapshots/restore", Some(&body)).is_none() {
                        return set_error(&app, "Pimpo could not prepare the snapshot of the data; nothing was changed");
                    }
                }
            }
            let older = app
                .updater_builder()
                .endpoints(vec![version_endpoint(&prev)])
                .map(|b| b.version_comparator(|cur, release| release.version != cur))
                .and_then(|b| b.build());
            let update = match older {
                Ok(u) => u.check().await,
                Err(e) => return set_error(&app, e),
            };
            match update {
                Ok(Some(u)) => match u.download_and_install(|_, _| {}, || {}).await {
                    Ok(()) => {
                        if let Some(p) = data_file(&app, "previous-version") {
                            let _ = std::fs::remove_file(p);
                        }
                        stop(&app);
                        app.restart();
                    }
                    Err(e) => set_error(&app, e),
                },
                Ok(None) => set_error(&app, format!("version {prev} is not available to install")),
                Err(e) => set_error(&app, e),
            }
        });
    }

    /// local_api calls this computer's Pimpo as its owner, over HTTP/1.0 so
    /// the answer is never chunked.
    fn local_api(app: &AppHandle, method: &str, path: &str, body: Option<&str>) -> Option<String> {
        use std::io::{Read, Write};
        let base = app.state::<Local>().0.lock().unwrap().clone()?;
        let token = app.state::<Token>().0.lock().unwrap().clone()?;
        let addr = base.strip_prefix("http://")?.to_string();
        let mut conn = TcpStream::connect_timeout(&addr.parse().ok()?, Duration::from_secs(3)).ok()?;
        conn.set_read_timeout(Some(Duration::from_secs(30))).ok()?;
        let body = body.unwrap_or("");
        let req = format!("{method} {path} HTTP/1.0\r\nHost: {addr}\r\nAuthorization: Bearer {token}\r\nContent-Type: application/json\r\nContent-Length: {}\r\n\r\n{body}", body.len());
        conn.write_all(req.as_bytes()).ok()?;
        let mut out = String::new();
        conn.read_to_string(&mut out).ok()?;
        let (head, rest) = out.split_once("\r\n\r\n")?;
        let ok = head.split_whitespace().nth(1).is_some_and(|code| code.starts_with('2'));
        ok.then(|| rest.to_string())
    }

    pub fn keep_running_on_close(ev: &WindowEvent, window: &tauri::Window) {
        if window.label() == "mascot" {
            if let WindowEvent::Moved(_) | WindowEvent::Resized(_) = ev {
                remember_corner(window);
            }
            return;
        }
        if window.label() != "main" {
            return;
        }
        if let WindowEvent::CloseRequested { api, .. } = ev {
            api.prevent_close();
            let _ = window.hide();
        }
    }
}

#[cfg(all(test, desktop))]
mod tests {
    use super::desktop::{check_link, language, same_origin, update_endpoint, version_endpoint, word};
    use tauri::Url;

    #[test]
    fn menus_follow_the_language() {
        assert_eq!(language("pt_BR.UTF-8"), "pt");
        assert_eq!(language("pt-BR"), "pt");
        assert_eq!(language("zh-Hans"), "zh");
        assert_eq!(language("en_US"), "en");
        assert_eq!(language("xx"), "en");
        assert_eq!(language(""), "en");
        assert_eq!(word("pt", "quit"), "Sair do Pimpo");
        assert_eq!(word("en", "quit"), "Quit Pimpo");
        assert_eq!(word("xx", "quit"), "Quit Pimpo");
        for lang in ["pt", "en", "es", "fr", "de", "it", "ja", "zh", "ko", "ru"] {
            for key in ["open", "remote", "local", "login", "mascot", "update", "install", "quit", "notLink", "https", "noToken", "badHome", "homeLocal", "noConfig"] {
                assert!(!word(lang, key).is_empty(), "{lang} {key}");
            }
            assert!(word(lang, "install").contains("{}"), "{lang} install needs the version");
        }
    }

    #[test]
    fn updates_come_from_the_project_releases() {
        assert_eq!(update_endpoint(false).as_str(), "https://github.com/turbine-dev/pimpo/releases/latest/download/latest.json");
        assert_eq!(update_endpoint(true).as_str(), "https://github.com/turbine-dev/pimpo/releases/download/channel-beta/latest.json");
        assert_eq!(version_endpoint("0.6.0-beta.2").as_str(), "https://github.com/turbine-dev/pimpo/releases/download/v0.6.0-beta.2/latest.json");
    }

    #[test]
    fn links_need_https_or_a_private_address_and_a_token() {
        assert!(check_link("https://pimpo.tail1.ts.net/auth?token=abc").is_ok());
        assert!(check_link("http://192.168.1.20:7788/auth?token=abc").is_ok());
        assert!(check_link("http://100.101.1.2:7788/auth?token=abc").is_ok());
        assert!(check_link("http://example.com/auth?token=abc").is_err());
        assert!(check_link("http://100.200.1.2/auth?token=abc").is_err());
        assert!(check_link("https://pimpo.example.com/auth").is_err());
        assert!(check_link("not a link").is_err());
    }

    #[test]
    fn only_this_computers_pimpo_is_local() {
        let base = "http://127.0.0.1:4321";
        let ok = |u: &str| same_origin(base, &Url::parse(u).unwrap());
        assert!(ok("http://127.0.0.1:4321/desktop/mascot?on=1"));
        assert!(ok("http://127.0.0.1:4321"));
        assert!(!ok("http://127.0.0.1:43210/desktop/mascot"));
        assert!(!ok("http://user:pw@127.0.0.1:4321/desktop/mascot"));
        assert!(!ok("https://127.0.0.1:4321/desktop/mascot"));
        assert!(!ok("http://localhost:4321/desktop/mascot"));
        assert!(!ok("http://127.0.0.1/desktop/mascot"));
        assert!(!same_origin("not a base", &Url::parse("http://127.0.0.1:4321").unwrap()));
    }
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let builder = tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_opener::init());

    #[cfg(desktop)]
    let builder = builder
        .plugin(tauri_plugin_single_instance::init(|app, _, _| desktop::show(app)))
        .plugin(tauri_plugin_autostart::init(tauri_plugin_autostart::MacosLauncher::LaunchAgent, None))
        .plugin(tauri_plugin_updater::Builder::new().build())
        .on_window_event(|w, ev| desktop::keep_running_on_close(ev, w))
        .invoke_handler(tauri::generate_handler![desktop::remote, desktop::use_remote, desktop::use_local])
        .setup(|app| {
            desktop::window(app.handle())?;
            desktop::tray(app.handle())?;
            desktop::start(app.handle())?;
            desktop::check_periodically(app.handle());
            Ok(())
        });

    #[cfg(mobile)]
    let builder = builder.setup(|app| {
        let conf = app.config().app.windows.iter().find(|w| w.label == "main").expect("main window config").clone();
        tauri::WebviewWindowBuilder::from_config(app.handle(), &conf)?.build()?;
        Ok(())
    });

    let app = builder
        .build(tauri::generate_context!())
        .expect("error while building Pimpo");
    app.run(|_app, _ev| {
        #[cfg(desktop)]
        match _ev {
            tauri::RunEvent::Exit => desktop::stop(_app),
            #[cfg(target_os = "macos")]
            tauri::RunEvent::Reopen { .. } => desktop::show(_app),
            _ => {}
        }
    });
}
