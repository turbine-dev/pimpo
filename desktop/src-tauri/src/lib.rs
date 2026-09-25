//! The desktop app runs the Vigia server as a sidecar on a free loopback
//! port and shows its web UI. Closing the window keeps the agent running in
//! the menu bar; only Quit stops it. On mobile there is no sidecar: the
//! shell page pairs with a Vigia running elsewhere.

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

    // The window is built here rather than from the config so links that
    // ask for a new window (target=_blank) open in the system browser; the
    // webview would otherwise swallow them.
    pub fn window(app: &AppHandle) -> tauri::Result<()> {
        let conf = app.config().app.windows.iter().find(|w| w.label == "main").expect("main window config").clone();
        let handle = app.clone();
        WebviewWindowBuilder::from_config(app, &conf)?
            .on_new_window(move |url, _| {
                if matches!(url.scheme(), "http" | "https" | "mailto") {
                    let _ = handle.opener().open_url(url.as_str(), None::<&str>);
                }
                NewWindowResponse::Deny
            })
            .build()?;
        Ok(())
    }

    pub fn show(app: &AppHandle) {
        if let Some(w) = app.get_webview_window("main") {
            let _ = w.show();
            let _ = w.unminimize();
            let _ = w.set_focus();
        }
    }

    fn status(app: &AppHandle, text: &str) {
        if let Some(w) = app.get_webview_window("main") {
            let js = format!(
                "document.getElementById('status') && (document.getElementById('status').textContent = {:?})",
                text
            );
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

    pub fn start(app: &AppHandle) -> Result<(), Box<dyn std::error::Error>> {
        let port = free_port()?;
        let token = token();
        let addr = format!("127.0.0.1:{port}");
        let (mut rx, child) = app
            .shell()
            .sidecar("vigia")?
            .args(["serve", "--addr", &addr])
            .env("VIGIA_TOKEN", &token)
            .env("VIGIA_EXIT_WITH_PARENT", "1")
            .env("VIGIA_DESKTOP_NOTIFY", "1")
            .spawn()?;
        app.manage(Server(Mutex::new(Some(child))));

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
                        status(&handle, &format!("O Vigia parou (código {:?}). Feche e abra de novo.", p.code));
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
            status(&handle, "O Vigia demorou demais para abrir.");
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
        let open = MenuItem::with_id(app, "open", "Abrir o Vigia", true, None::<&str>)?;
        let at_login = app.autolaunch().is_enabled().unwrap_or(false);
        let login = CheckMenuItem::with_id(app, "login", "Abrir ao iniciar o computador", true, at_login, None::<&str>)?;
        let quit = MenuItem::with_id(app, "quit", "Sair do Vigia", true, Some("CmdOrCtrl+Q"))?;
        let menu = Menu::with_items(app, &[&open, &login, &PredefinedMenuItem::separator(app)?, &quit])?;
        TrayIconBuilder::with_id("vigia")
            .icon(tauri::image::Image::from_bytes(include_bytes!("../icons/tray.png"))?)
            .icon_as_template(true)
            .tooltip("Vigia")
            .menu(&menu)
            .show_menu_on_left_click(false)
            .on_menu_event(move |app, ev| match ev.id().as_ref() {
                "open" => show(app),
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
        .expect("error while building Vigia");
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
