# Evaluation

The critical part is the evaluation. We will use a pattern (such as pi-autoresearch) to do the following: 

Given a technical document, LLMs analyze the document to determine whether it clearly specifies: 
- The standards to flow 
- Which standard, the specific requirements (such as "the max pressure should not exceed xyz"), the corresponding section and page number of the requirement. 
- Check whether the doc misses any standard that is relevant to the doc. 
- Check whether the references are correct (assume my system has a rich set of full-searchable standards). 
- Pay special attentions to the metrics, such as the max/min working temperatures, pressures, speeds, size, etc. 

To evaluate, I am assuming that I should work out a few docs, identify all the related standards, the corresponding requirements/metrics, and where they should be referenced in the docs. Then run the loop.