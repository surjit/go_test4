const option = {
    cursorBlink: true,
    rendererType: "canvas",
    fontFamily: 'Consolas, Lucida Console, Courier New, monospace',
    fontSize: '13px'
};

const terminal = new Terminal(option);
const fitAddon = new FitAddon.FitAddon();
const linkAddon = new WebLinksAddon.WebLinksAddon();

terminal.loadAddon(fitAddon);
terminal.loadAddon(linkAddon);
terminal.open(document.getElementById('terminal'));
terminal.focus();
fitAddon.fit();

const socket = new WebSocket('ws://' + window.location.host + '/terminal-ws/' + uuid)

// workaround
// for redraw terminal screen when reload window
const redraw = (socket, msg) => {
    msg.data.cols--
    terminal.resize(msg.data.cols, msg.data.rows)
    socket.send(JSON.stringify(msg));

    msg.data.cols++
    terminal.resize(msg.data.cols, msg.data.rows)
    socket.send(JSON.stringify(msg));
}

socket.onopen = () => {
    const msg = {
        event: "resize",
        session: uuid,
        data: {
            "cols": terminal.cols,
            "rows": terminal.rows,
        },
    };
    socket.send(JSON.stringify(msg));

    redraw(socket, msg)

    terminal.onData(data => {
        switch (socket.readyState) {
            case WebSocket.CLOSED:
            case WebSocket.CLOSING:
                terminal.dispose();
                return;
        }
        const msg = {
            event: "sendKey",
            session: uuid,
            data: data,
        }
        socket.send(JSON.stringify(msg));
    })

    socket.onclose = () => {
        terminal.writeln('[Disconnected]');
    }

    socket.onmessage = (e) => {
        terminal.write(e.data);
    }

    terminal.onResize((size) => {
        terminal.resize(size.cols, size.rows);
        const msg = {
            event: "resize",
            session: uuid,
            data: {
                cols: size.cols,
                rows: size.rows,
            },
        }
        socket.send(JSON.stringify(msg));
    });

    window.onbeforeunload = () => {
        socket.close();
    }

    window.addEventListener("resize", () => {
        fitAddon.fit()
    })
}
