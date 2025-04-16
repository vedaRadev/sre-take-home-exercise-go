package main

import (
	"sync"
	"log"
	"fmt"
)

// TODO The SyncedPrinter API is terrible and quickly-made. Clean this up.
type SyncedPrinter struct   {
	mu sync.Mutex
	debugEnabled bool
}

func (sp *SyncedPrinter) Lock() { sp.mu.Lock() }
func (sp *SyncedPrinter) Unlock() { sp.mu.Unlock() }

func (sp *SyncedPrinter) Logf(format string, params ...any) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	log.Printf(format, params...)
}

func (sp *SyncedPrinter) LogfBypassLock(format string, params ...any) {
	log.Printf(format, params...)
}

func (sp *SyncedPrinter) LogLine(line string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	log.Println(line)
}

func (sp *SyncedPrinter) LogLineBypassLock(line string) {
	log.Println(line)
}

func (sp *SyncedPrinter) Printf(format string, params ...any) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	fmt.Printf(format, params...)
}

func (sp *SyncedPrinter) PrintfBypassLock(format string, params ...any) {
	fmt.Printf(format, params...)
}

func (sp *SyncedPrinter) Println(line string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	fmt.Println(line)
}

func (sp *SyncedPrinter) PrintlnBypassLock(line string) {
	fmt.Println(line)
}

// DEBUG LOGGING/PRINTING

func (sp *SyncedPrinter) DebugLock() {
	if sp.debugEnabled {
		sp.Lock()
	}
}

func (sp *SyncedPrinter) DebugUnlock() {
	if sp.debugEnabled {
		sp.Unlock()
	}
}

func (sp *SyncedPrinter) DebugLogf(format string, params ...any) {
	if sp.debugEnabled {
		sp.Logf(format, params...)
	}
}

func (sp *SyncedPrinter) DebugLogfBypassLock(format string, params ...any) {
	if sp.debugEnabled {
		sp.LogfBypassLock(format, params...)
	}
}

func (sp *SyncedPrinter) DebugLogLine(line string) {
	if sp.debugEnabled {
		sp.LogLine(line)
	}
}

func (sp *SyncedPrinter) DebugLogLineBypassLock(line string) {
	if sp.debugEnabled {
		sp.LogLineBypassLock(line)
	}
}

func (sp *SyncedPrinter) DebugPrintf(format string, params ...any) {
	if sp.debugEnabled {
		sp.Printf(format, params...)
	}
}

func (sp *SyncedPrinter) DebugPrintfBypassLock(format string, params ...any) {
	if sp.debugEnabled {
		sp.PrintfBypassLock(format, params...)
	}
}

func (sp *SyncedPrinter) DebugPrintln(line string) {
	if sp.debugEnabled {
		sp.Println(line)
	}
}

func (sp *SyncedPrinter) DebugPrintlnBypassLock(line string) {
	if sp.debugEnabled {
		sp.PrintlnBypassLock(line)
	}
}


