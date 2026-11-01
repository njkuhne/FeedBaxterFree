import { useEffect, useId, useImperativeHandle, useRef, useState, type KeyboardEvent, type Ref } from "react";
import { Button } from "../components/Button";
import { TagPill } from "../components/TagPill";
import { TextField } from "../components/TextField";
import { plural } from "../lib/format";
import type { TagCount } from "../lib/types";
import { TAG_ERROR, validTag } from "./statusCopy";

export interface TagEditorHandle {
  open: () => void;
}

interface TagEditorProps {
  /** Tags on the item (detail). Omit in add-only mode (bulk). */
  tags?: string[];
  suggestions: TagCount[];
  onAdd: (name: string) => Promise<boolean>;
  onRemove?: (name: string) => void;
  /** Start with the input open (bulk popover). */
  initiallyOpen?: boolean;
  ref?: Ref<TagEditorHandle>;
}

const MAX_OPTIONS = 8;

/** Existing tags not on the item, containing the typed text, most used first. */
function optionsFor(suggestions: TagCount[], have: string[], text: string): TagCount[] {
  const taken = new Set(have.map((t) => t.toLowerCase()));
  const needle = text.trim().toLowerCase();
  return suggestions
    .filter((s) => !taken.has(s.name.toLowerCase()) && s.name.toLowerCase().includes(needle))
    .sort((a, b) => b.count - a.count || a.name.localeCompare(b.name))
    .slice(0, MAX_OPTIONS);
}

/** Pills for the tags on an item, and an add field with an ARIA combobox of existing tags. */
export function TagEditor({ tags, suggestions, onAdd, onRemove, initiallyOpen = false, ref }: TagEditorProps) {
  const [open, setOpen] = useState(initiallyOpen);
  const [text, setText] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [listOpen, setListOpen] = useState(true);
  const [active, setActive] = useState(-1);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const addRef = useRef<HTMLButtonElement | null>(null);
  const returnFocus = useRef(false);
  const adding = useRef(false);
  const listId = useId();
  const addOnly = tags === undefined;
  const have = tags ?? [];

  useImperativeHandle(ref, () => ({
    open: () => {
      setOpen(true);
      setListOpen(true);
      inputRef.current?.focus();
    },
  }));

  useEffect(() => {
    if (open) inputRef.current?.focus();
    else if (returnFocus.current) {
      returnFocus.current = false;
      addRef.current?.focus();
    }
  }, [open]);

  const options = optionsFor(suggestions, have, text);
  const showList = open && listOpen && options.length > 0;
  const optionId = (i: number) => `${listId}-opt-${i}`;
  const activeIndex = showList && active < options.length ? active : -1;

  const closeInput = () => {
    returnFocus.current = true;
    setOpen(false);
    setText("");
    setError(null);
    setActive(-1);
  };

  const submit = async (raw: string) => {
    if (adding.current) return;
    const valid = validTag(raw);
    if (valid === null) {
      setError(TAG_ERROR);
      return;
    }
    setError(null);
    if (have.some((t) => t.toLowerCase() === valid.toLowerCase())) {
      setText("");
      setActive(-1);
      return;
    }
    adding.current = true;
    try {
      if (await onAdd(valid)) {
        setText("");
        setActive(-1);
        setListOpen(true);
      }
    } finally {
      adding.current = false;
    }
    inputRef.current?.focus();
  };

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    const n = options.length;
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        if (n === 0) return;
        setListOpen(true);
        setActive(!showList || activeIndex < 0 ? 0 : (activeIndex + 1) % n);
        return;
      case "ArrowUp":
        e.preventDefault();
        if (n === 0) return;
        setListOpen(true);
        setActive(!showList || activeIndex < 0 ? n - 1 : (activeIndex - 1 + n) % n);
        return;
      case "Enter": {
        e.preventDefault();
        e.stopPropagation();
        const chosen = activeIndex >= 0 ? options[activeIndex] : undefined;
        void submit(chosen !== undefined ? chosen.name : text);
        return;
      }
      case "Escape":
        if (showList) {
          e.preventDefault();
          e.stopPropagation();
          setListOpen(false);
          setActive(-1);
        } else if (addOnly) {
          if (text !== "") {
            e.preventDefault();
            e.stopPropagation();
            setText("");
            setError(null);
          }
          // An empty input with the list closed: let it bubble, the host closes itself.
        } else {
          e.preventDefault();
          e.stopPropagation();
          closeInput();
        }
        return;
      case "Tab":
        setListOpen(false);
        setActive(-1);
        return;
    }
  };

  return (
    <div className="tag-editor">
      {tags !== undefined && onRemove !== undefined
        ? tags.map((t) => <TagPill key={t} name={t} onRemove={() => onRemove(t)} />)
        : null}
      {open ? (
        <div className="tag-combo">
          <TextField
            label="Add tag"
            hideLabel
            placeholder="Tag name"
            autoComplete="off"
            className="tag-input"
            ref={inputRef}
            value={text}
            error={error}
            role="combobox"
            aria-expanded={showList}
            aria-controls={listId}
            aria-autocomplete="list"
            aria-activedescendant={activeIndex >= 0 ? optionId(activeIndex) : undefined}
            onChange={(e) => {
              setText(e.target.value);
              setError(null);
              setActive(-1);
              setListOpen(true);
            }}
            onKeyDown={onKeyDown}
            onBlur={(e) => {
              if (!(e.relatedTarget instanceof Node) || e.currentTarget.closest(".tag-combo")?.contains(e.relatedTarget) !== true) {
                setListOpen(false);
                setActive(-1);
              }
            }}
            onFocus={() => setListOpen(true)}
          />
          <ul id={listId} role="listbox" aria-label="Existing tags" className="tag-options" hidden={!showList}>
            {options.map((o, i) => (
              <li
                key={o.name}
                id={optionId(i)}
                role="option"
                aria-selected={i === activeIndex}
                aria-label={`${o.name}, ${plural(o.count, "item", "items")}`}
                className={i === activeIndex ? "tag-option tag-option-active" : "tag-option"}
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => void submit(o.name)}
              >
                <span>{o.name}</span>
                <span className="tag-option-count">{o.count}</span>
              </li>
            ))}
          </ul>
        </div>
      ) : (
        <Button
          variant="text"
          ref={addRef}
          className="tag-add"
          onClick={() => {
            setOpen(true);
            setListOpen(true);
          }}
        >
          + Add tag
        </Button>
      )}
    </div>
  );
}
