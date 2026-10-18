# This base is already available on onto; Debian Bullseye supplies glibc 2.31.
FROM nikolaik/python-nodejs:python3.12-nodejs20-bullseye@sha256:63b385e48df3d9fb56fc36d5c6371a0697cc140037b3966ef162458aa6b0276c

ARG PIP_INDEX_URL=https://mirrors.aliyun.com/pypi/simple
RUN python -m pip install --no-cache-dir --index-url https://download.pytorch.org/whl/cpu \
      torch==2.7.1+cpu torchvision==0.22.1+cpu

COPY mineru /opt/mineru
# PDF workers do not use OpenCV's Qt/OpenGL UI. Select its headless distribution
# in package metadata so pip check also reflects the installed server runtime.
RUN sed -i 's/"opencv-python>=/"opencv-python-headless>=/' /opt/mineru/pyproject.toml
RUN python -m pip install --no-cache-dir /opt/mineru \
      'transformers>=5.10.1,<6' 'accelerate>=1.5.1' 'safetensors>=0.4,<1' \
      'psycopg2-binary>=2.9' 'PyMuPDF>=1.24' 'nats-py>=2.8' \
    && python -m pip check
COPY worker /opt/pdf-parser
RUN mkdir -p /home/gui && chown 1001:1001 /home/gui
WORKDIR /opt/pdf-parser
ENV HOME=/home/gui \
    TZ=Asia/Shanghai \
    OMP_NUM_THREADS=8 \
    MKL_NUM_THREADS=8 \
    OPENBLAS_NUM_THREADS=8 \
    PYTHONUNBUFFERED=1 \
    PYTHONPATH=/opt/pdf-parser \
    MINERU_CLI=/usr/local/bin/mineru-kit \
    MINERU_PYTHON=/usr/local/bin/python \
    MINERU_CONFIG=/home/gui/.mineru/config.yaml \
    MINERU_DEVICE_MODE=cpu \
    MINERU_MODEL_SMALL_BACKEND=onnx \
    MINERU_MODEL_VLM_ENGINE=llama-cpp
CMD ["python", "pdf_parser.py"]
