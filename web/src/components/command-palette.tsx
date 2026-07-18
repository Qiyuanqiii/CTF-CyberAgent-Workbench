import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { Command as CommandIcon, Search, X } from "lucide-react";

export interface CommandPaletteCommand {
  id: string;
  label: string;
  group: string;
  keywords?: string[];
  run: () => void;
}

export function CommandPalette({ commands }: { commands: CommandPaletteCommand[] }) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const filtered = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase();
    if (!needle) return commands;
    return commands.filter((command) => [command.label, command.group,
      ...(command.keywords ?? [])].some((value) => value.toLocaleLowerCase().includes(needle)));
  }, [commands, query]);

  useEffect(() => {
    const shortcut = (event: globalThis.KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && !event.altKey && event.key.toLocaleLowerCase() === "k") {
        event.preventDefault();
        setOpen((current) => !current);
      }
    };
    globalThis.addEventListener("keydown", shortcut);
    return () => globalThis.removeEventListener("keydown", shortcut);
  }, []);

  useEffect(() => {
    if (open) {
      setQuery("");
      setSelected(0);
      globalThis.requestAnimationFrame(() => inputRef.current?.focus());
    }
  }, [open]);

  useEffect(() => {
    setSelected((current) => Math.min(current, Math.max(0, filtered.length - 1)));
  }, [filtered.length]);

  const execute = (command: CommandPaletteCommand | undefined) => {
    if (!command) return;
    setOpen(false);
    command.run();
  };
  const navigate = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Escape") {
      event.preventDefault();
      setOpen(false);
    } else if (event.key === "ArrowDown" && filtered.length > 0) {
      event.preventDefault();
      setSelected((current) => (current + 1) % filtered.length);
    } else if (event.key === "ArrowUp" && filtered.length > 0) {
      event.preventDefault();
      setSelected((current) => (current - 1 + filtered.length) % filtered.length);
    } else if (event.key === "Enter") {
      event.preventDefault();
      execute(filtered[selected]);
    }
  };

  return <>
    <button aria-label="Open command palette" className="icon-button"
      onClick={() => setOpen(true)} title="Command palette" type="button">
      <CommandIcon aria-hidden="true" size={16} />
    </button>
    {open && <div className="command-palette-backdrop" onMouseDown={(event) => {
      if (event.currentTarget === event.target) setOpen(false);
    }}>
      <section aria-label="Command palette" aria-modal="true" className="command-palette"
        role="dialog">
        <header>
          <Search aria-hidden="true" size={17} />
          <input aria-activedescendant={filtered[selected] ? `command-${filtered[selected].id}` : undefined}
            aria-controls="command-palette-results" aria-label="Find a command"
            onChange={(event) => setQuery(event.target.value)} onKeyDown={navigate}
            placeholder="Find a command" ref={inputRef} type="search" value={query} />
          <button aria-label="Close command palette" className="icon-button"
            onClick={() => setOpen(false)} title="Close" type="button">
            <X aria-hidden="true" size={16} />
          </button>
        </header>
        <div className="command-palette-results" id="command-palette-results" role="listbox">
          {filtered.map((command, index) => <button aria-selected={selected === index}
            className={selected === index ? "selected" : ""} id={`command-${command.id}`}
            key={command.id} onClick={() => execute(command)} onMouseEnter={() => setSelected(index)}
            role="option" type="button">
            <span>{command.label}</span><small>{command.group}</small>
          </button>)}
          {filtered.length === 0 && <div className="command-palette-empty">No matching command</div>}
        </div>
      </section>
    </div>}
  </>;
}
