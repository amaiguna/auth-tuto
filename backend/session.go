package main

// HACK: スレッドセーフでないのでsync.Mapに書き換え(必要なら)
var sessions = map[string]sessionData{}
