#pragma once

#include <stddef.h>

#ifdef __cplusplus
extern "C"
{
#endif

    void *GetDisplay();
    int CycleRefreshRate(void *);
    int GetCurrentRefreshRate(void *);
    void ReleaseDisplay(void *);

#ifdef __cplusplus
}
#endif