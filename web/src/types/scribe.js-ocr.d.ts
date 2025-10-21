declare module 'scribe.js-ocr' {
  const scribe: {
    extractText: (urls: string[]) => Promise<Record<string, string>>;
  };
  export default scribe;
}
