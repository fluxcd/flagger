"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
const emojiData = require('markdown-it-emoji/lib/data/full.json');
exports.default = (str) => {
    return String(str).replace(/:(.+?):/g, (placeholder, key) => emojiData[key] || placeholder);
};
