//! The desktop app runs the Zodim server as a sidecar on a free loopback
//! port and shows its web UI. Closing the window keeps the agent running in
//! the menu bar; only Quit stops it. It can instead open a Zodim running
//! elsewhere (a home server, a VPS), with the same link the phone pairs
//! with; the local server then stays off, so one bot and one set of
//! routines never run twice. On mobile there is no sidecar: the shell page
//! always pairs with a Zodim running elsewhere.

#[cfg(desktop)]
mod desktop {
    use std::net::{SocketAddr, TcpListener, TcpStream};
    use std::sync::Mutex;
    use std::time::{Duration, Instant};

    use tauri::menu::{CheckMenuItem, Menu, MenuItem, PredefinedMenuItem};
    use tauri::tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent};
    use tauri::webview::NewWindowResponse;
    use tauri::{AppHandle, Manager, Url, WebviewWindowBuilder, WindowEvent};
    use tauri_plugin_opener::OpenerExt;
    use tauri_plugin_autostart::ManagerExt as _;
    use tauri_plugin_shell::process::{CommandChild, CommandEvent};
    use tauri_plugin_shell::ShellExt;

    pub struct Server(pub Mutex<Option<CommandChild>>);

    /// Shell is the address of the app's own page, to come back to it from
    /// a remote Zodim.
    pub struct Shell(pub Mutex<Option<Url>>);

    fn remote_file(app: &AppHandle) -> Option<std::path::PathBuf> {
        app.path().app_config_dir().ok().map(|d| d.join("remote.txt"))
    }

    /// The remote Zodim chosen, as its link and optional home address.
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
        let url = Url::parse(link).map_err(|_| "Isso não parece um link.".to_string())?;
        let host = url.host_str().unwrap_or("");
        match url.scheme() {
            "https" => {}
            "http" if private_host(host) => {}
            _ => return Err("Use https: o token não pode viajar em aberto.".into()),
        }
        if !url.query_pairs().any(|(k, v)| k == "token" && !v.is_empty()) {
            return Err("Falta o token no link.".into());
        }
        Ok(url)
    }

    #[tauri::command]
    pub fn remote(app: AppHandle) -> Vec<String> {
        saved_remote(&app).map(|(l, h)| vec![l, h]).unwrap_or_default()
    }

    /// use_remote keeps the link and stops the local Zodim.
    #[tauri::command]
    pub fn use_remote(app: AppHandle, link: String, home: String) -> Result<(), String> {
        check_link(&link)?;
        if !home.is_empty() {
            let h = Url::parse(&home).map_err(|_| "Endereço de casa inválido.".to_string())?;
            if h.scheme() != "http" || !private_host(h.host_str().unwrap_or("")) {
                return Err("O endereço de casa precisa ser da rede local.".into());
            }
        }
        let file = remote_file(&app).ok_or("Sem pasta de configuração.")?;
        if let Some(dir) = file.parent() {
            std::fs::create_dir_all(dir).map_err(|e| e.to_string())?;
        }
        std::fs::write(&file, format!("{link}\n{home}\n")).map_err(|e| e.to_string())?;
        stop(&app);
        Ok(())
    }

    /// use_local forgets the remote Zodim and starts the one on this
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
        let w = WebviewWindowBuilder::from_config(app, &conf)?
            .initialization_script(format!("window.__ZODIM_DESKTOP__ = {platform:?}"))
            .on_new_window(move |url, _| {
                if matches!(url.scheme(), "http" | "https" | "mailto") {
                    let _ = handle.opener().open_url(url.as_str(), None::<&str>);
                }
                NewWindowResponse::Deny
            })
            .build()?;
        app.manage(Shell(Mutex::new(w.url().ok())));
        app.manage(Server(Mutex::new(None)));
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
            let js = format!("window.__zodimStatus && window.__zodimStatus({key:?}, {arg:?})");
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

    /// start runs the local Zodim unless a remote one was chosen; the shell
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
        let addr = format!("127.0.0.1:{port}");
        let (mut rx, child) = app
            .shell()
            .sidecar("zodim")?
            .args(["serve", "--addr", &addr])
            .env("ZODIM_TOKEN", &token)
            .env("ZODIM_EXIT_WITH_PARENT", "1")
            .env("ZODIM_DESKTOP_NOTIFY", "1")
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
                        // Stopped on purpose when switching to a remote Zodim.
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
        let open = MenuItem::with_id(app, "open", "Abrir o Zodim", true, None::<&str>)?;
        let remote = MenuItem::with_id(app, "remote", "Conectar a outro Zodim…", true, None::<&str>)?;
        let local = MenuItem::with_id(app, "local", "Usar o Zodim deste computador", true, None::<&str>)?;
        let at_login = app.autolaunch().is_enabled().unwrap_or(false);
        let login = CheckMenuItem::with_id(app, "login", "Abrir ao iniciar o computador", true, at_login, None::<&str>)?;
        let quit = MenuItem::with_id(app, "quit", "Sair do Zodim", true, Some("CmdOrCtrl+Q"))?;
        let menu = Menu::with_items(app, &[&open, &login, &PredefinedMenuItem::separator(app)?, &remote, &local, &PredefinedMenuItem::separator(app)?, &quit])?;
        TrayIconBuilder::with_id("zodim")
            .icon(tauri::image::Image::from_bytes(include_bytes!("../icons/tray.png"))?)
            .icon_as_template(true)
            .tooltip("Zodim")
            .menu(&menu)
            .show_menu_on_left_click(false)
            .on_menu_event(move |app, ev| match ev.id().as_ref() {
                "open" => show(app),
                "remote" => pair(app),
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

    pub fn keep_running_on_close(ev: &WindowEvent, window: &tauri::Window) {
        if let WindowEvent::CloseRequested { api, .. } = ev {
            api.prevent_close();
            let _ = window.hide();
        }
    }
}

#[cfg(all(test, desktop))]
mod tests {
    use super::desktop::check_link;

    #[test]
    fn links_need_https_or_a_private_address_and_a_token() {
        assert!(check_link("https://zodim.tail1.ts.net/auth?token=abc").is_ok());
        assert!(check_link("http://192.168.1.20:7788/auth?token=abc").is_ok());
        assert!(check_link("http://100.101.1.2:7788/auth?token=abc").is_ok());
        assert!(check_link("http://example.com/auth?token=abc").is_err());
        assert!(check_link("http://100.200.1.2/auth?token=abc").is_err());
        assert!(check_link("https://zodim.example.com/auth").is_err());
        assert!(check_link("not a link").is_err());
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
        .on_window_event(|w, ev| desktop::keep_running_on_close(ev, w))
        .invoke_handler(tauri::generate_handler![desktop::remote, desktop::use_remote, desktop::use_local])
        .setup(|app| {
            desktop::window(app.handle())?;
            desktop::tray(app.handle())?;
            desktop::start(app.handle())?;
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
        .expect("error while building Zodim");
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
